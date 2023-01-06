// package main implements a program that generates a compile_commands.json file
// in the current Bazel directory.
package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"runtime"
	"sort"
	"strings"
)

// These two types are a minimal subset of the types from
// https://github.com/bazelbuild/bazel/blob/68e14b553e746655b71aaa59b766b659888f08b6/src/main/protobuf/analysis.proto
// The identifiers are encoded as json ints, not strings. Not sure if that's
// a bug or not.

type actionGraphContainer struct {
	Targets       []target
	Actions       []action
	DepSetOfFiles []depSetOfFiles
}

type target struct {
	ID    int `json:"id"`
	Label string
}

type action struct {
	TargetID        int `json:"targetId"`
	ConfigurationID int `json:"configurationId"`
	Mnemonic        string
	Arguments       []string
}

type depSetOfFiles struct {
	ID                int `json:"id"`
	DirectArtifactIds []int
}

// type derived from compile_commands.json format

type compileCommand struct {
	Directory string   `json:"directory"`
	Arguments []string `json:"arguments"`
	File      string   `json:"file"`
}

// internal types

type ccTarget struct {
	srcs  []string
	args  []string
	label string
}

//go:embed src_paths.cquery.bzl
var srcPathsCquerySrc []byte

// current workspace directory
var workspace string = os.Getenv("BUILD_WORKSPACE_DIRECTORY")

func getBazelInfo(v string) string {
	out := new(strings.Builder)
	cmd := exec.Command("bazel", "info", "workspace")
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	cmd.Dir = workspace
	if err := cmd.Run(); err != nil {
		panic(fmt.Errorf("could not get %q: %s", v, err))
	}
	return strings.TrimSpace(out.String())
}

func getXcodeSDKPath(dir string, sdk string) string {
	out := new(strings.Builder)
	cmd := exec.Command("xcrun", "--sdk", sdk, "--show-sdk-path")
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		panic(fmt.Errorf("could not get sdk path: %s", err))
	}
	return strings.TrimSpace(out.String())
}

func getXcodeDeveloperDir(dir string) string {
	out := new(strings.Builder)
	cmd := exec.Command("xcode-select", "-p")
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		panic(fmt.Errorf("could not get xcode developer directory: %s", err))
	}
	return strings.TrimSpace(out.String())
}

func main() {
	// determine the workspace path if it's not set already
	if workspace == "" {
		workspace = getBazelInfo("workspace")
	}
	executionRoot := getBazelInfo("execution_root")
	outputBaseDir := getBazelInfo("output_base")
	binDir := getBazelInfo("bazel-bin")

	var xcodeSDKPath string
	var xcodeDeveloperDir string
	switch runtime.GOOS {
	case "darwin":
		xcodeSDKPath = getXcodeSDKPath(executionRoot, "macosx")
		xcodeDeveloperDir = getXcodeDeveloperDir(executionRoot)
	}

	targetLabels := map[int]string{}
	ccTargets := map[string]*ccTarget{}

	queryMnemonic := func(n string) {

		out := new(strings.Builder)
		cmd := exec.Command(
			"bazel",
			"aquery",
			fmt.Sprintf(`mnemonic("%s", //...)`, n),
			"--output=jsonproto",
		)
		cmd.Stderr = os.Stderr
		cmd.Stdout = out
		cmd.Dir = workspace

		if err := cmd.Run(); err != nil {
			panic(fmt.Errorf("failed to run Bazel: %s", err))
		}

		var container actionGraphContainer
		if err := json.Unmarshal([]byte(out.String()), &container); err != nil {
			panic(fmt.Errorf("failed to parse aquery output: %s", err))
		}

		for _, target := range container.Targets {
			targetLabels[target.ID] = target.Label
		}

		for _, action := range container.Actions {
			if action.Mnemonic != n {
				continue
			}
			label, ok := targetLabels[action.TargetID]
			if !ok {
				panic(fmt.Errorf("missing label (%d) in aquery output", action.TargetID))
			}
			var args []string
			switch n {
			case "ObjcCompile":
				args = []string{"clang", "-xobjective-c++"}
			case "CppCompile":
				args = []string{"clang", "-xc++"}
			}
			for i := 1; i < len(action.Arguments); i++ {
				arg := action.Arguments[i]
				switch {
				case arg == "-c":
					i++
					continue
				case strings.HasPrefix(arg, "-Ibazel-out"):
					arg = "-I" + path.Join(outputBaseDir, strings.TrimPrefix(arg, "-I"))
				case strings.HasPrefix(arg, "external/") ||
					strings.HasPrefix(arg, "bazel-out"):
					arg = path.Join(outputBaseDir, arg)
				}
				switch runtime.GOOS {
				case "darwin":
					arg = strings.ReplaceAll(arg, "__BAZEL_XCODE_SDKROOT__", xcodeSDKPath)
					arg = strings.ReplaceAll(arg, "__BAZEL_XCODE_DEVELOPER_DIR__", xcodeDeveloperDir)
				}
				args = append(args, arg)
			}
			ccTargets[label] = &ccTarget{
				args: args,
			}
		}
	}

	queryMnemonic("CppCompile")
	queryMnemonic("ObjcCompile")

	labels := make(sort.StringSlice, len(ccTargets))
	{
		var i int
		for label := range ccTargets {
			labels[i] = label
			i++
		}
		labels.Sort()
	}

	// write src paths to temporary file
	tmpDir, err := os.MkdirTemp("", "cquery")
	if err != nil {
		panic(fmt.Errorf("failed to create temporary directory: %s", err))
	}
	defer os.RemoveAll(tmpDir)
	cqueryPath := path.Join(tmpDir, "src_cquery.bzl")
	if err := os.WriteFile(cqueryPath, srcPathsCquerySrc, 0777); err != nil {
		panic(fmt.Errorf("failed to write cquery file: %s", err))
	}

	scannedSrcs := map[string]bool{}
	for _, label := range labels {
		cmd := exec.Command(
			"bazel",
			"cquery",
			fmt.Sprintf(`kind("source file", deps(%s))`, label),
			"--output",
			"starlark",
			"--starlark:file",
			cqueryPath,
		)
		stderr := new(strings.Builder)
		stdout := new(strings.Builder)
		cmd.Stderr = stderr
		cmd.Stdout = stdout
		cmd.Dir = workspace
		if err := cmd.Run(); err != nil {
			panic(fmt.Errorf("failed to query source paths of %q\n\n%s", label, stderr))
		}
		fmt.Println(label)
		var srcs []string
		scn := bufio.NewScanner(strings.NewReader(stdout.String()))
		for scn.Scan() {
			txt := scn.Text()
			if txt == "" {
				continue
			}
			if _, ok := scannedSrcs[txt]; ok {
				continue
			}
			scannedSrcs[txt] = true
			srcs = append(srcs, txt)
		}
		if err := scn.Err(); err != nil {
			panic(fmt.Errorf("%s\n\nfailed to parse output of bazel cquery: %s", stderr, err))
		}
		ccTargets[label].srcs = srcs
	}

	var compileCommands []compileCommand
	for _, label := range labels {
		target := ccTargets[label]
		for _, src := range target.srcs {
			compileCommands = append(compileCommands, compileCommand{
				Directory: workspace,
				File:      src,
				Arguments: append(target.args,
					"-iquote",
					binDir,
					"-iquote",
					executionRoot,
					"-iquote",
					outputBaseDir,
					src,
				),
			})
		}
	}

	content, err := json.MarshalIndent(&compileCommands, "", "  ")
	if err != nil {
		panic(err)
	}

	err = ioutil.WriteFile(
		path.Join(workspace, "compile_commands.json"),
		content,
		0644,
	)
	if err != nil {
		panic(err)
	}
}
