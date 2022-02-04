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
	Directory string `json:"directory"`
	Command   string `json:"command"`
	File      string `json:"file"`
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

func main() {
	// determine the workspace path if it's not set already
	if workspace == "" {
		workspace = getBazelInfo("workspace")
	}
	executionRoot := getBazelInfo("execution_root")
	outputBaseDir := getBazelInfo("output_base")
	binDir := getBazelInfo("bazel-bin")

	out := new(strings.Builder)
	cmd := exec.Command(
		"bazel",
		"aquery",
		`mnemonic("CppCompile", //...)`,
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

	targetLabels := map[int]string{}
	for _, target := range container.Targets {
		targetLabels[target.ID] = target.Label
	}

	ccTargets := map[string]*ccTarget{}
	for _, action := range container.Actions {
		if action.Mnemonic != "CppCompile" {
			continue
		}
		label, ok := targetLabels[action.TargetID]
		if !ok {
			panic(fmt.Errorf("missing label (%d) in aquery output", action.TargetID))
		}
		var args []string
		for i := 0; i < len(action.Arguments); i++ {
			arg := action.Arguments[i]
			switch {
			case arg == "-c":
				i++
				continue
			case strings.HasPrefix(arg, "external/"):
				arg = path.Join(outputBaseDir, arg)
			}
			args = append(args, arg)
		}
		ccTargets[label] = &ccTarget{
			args: args,
		}
	}

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
				Command: strings.Join(append(target.args,
					"-iquote",
					binDir,
					"-iquote",
					executionRoot,
					"-iquote",
					outputBaseDir,
					"-xc++",
					src,
				), " "),
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
