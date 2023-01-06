# Bazel Compilation Database

Generate a compile_commands.json file at the root of the given Bazel workspace.

The command directly calls Bazel, queries to find all targets that emit a
`CppCompile` action and collects the compilation arguments for that action.
It then queries Bazel for a list of sources of each target, and accociates the
information retreived from the compile action with that file in the format
expected by compile_commands.json.

I use this, but it is untested. There are some tested alternatives:

 - https://github.com/grailbio/bazel-compilation-database
 - https://github.com/hedronvision/bazel-compile-commands-extractor

## Building with Go

Ensure you have at least Go 1.17 installed.

`go build generate-compile-commands.go`

## Building with Bazel

`bazel build :generate_compile_commands`

## Running from your Bazel workspace

You can install the binary in your path and run it from any Bazel workspace. Alternatively,
add this repository to your Bazel workspace and run it via Bazel.

```

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

# Rules Go https://github.com/bazelbuild/rules_go

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "d6b2513456fe2229811da7eb67a444be7785f5323c6708b38d851d2b51e54d83",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.30.0/rules_go-v0.30.0.zip",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.30.0/rules_go-v0.30.0.zip",
    ],
)

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")

go_rules_dependencies()

go_register_toolchains(version = "1.17.6")

# Bazel compile commands https://github.com/chriscraws/bazel-compile-commands

http_archive(
    name = "bazel_compile_commands",
    url = "https://github.com/chriscraws/bazel-compile-commands/archive/main.zip",
    strip_prefix = "bazel-compile-commands-main",
)
```

Then you can run `bazel run @bazel_compile_commands//:generate_compile_commands` from anywhere in your workspace.

## Glossary

 - [Compilation Database](https://clang.llvm.org/docs/JSONCompilationDatabase.html)
 - [Bazel](https://bazel.build/)
 - [Bazel Action Query](https://docs.bazel.build/versions/main/aquery.html)
