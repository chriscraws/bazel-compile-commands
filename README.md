# Bazel Compilation Database

Generate a compile_commands.json file at the root of the given Bazel workspace.

The command directly calls Bazel, queries to find all targets that emit a
`CppCompile` action and collects the compilation arguments for that action.
It then queries Bazel for a list of sources of each target, and accociates the
information retreived from the compile action with that file in the format
expected by compile_commands.json.

I use this, but it is untested. There are some tested alternatives:

https://github.com/grailbio/bazel-compilation-database
https://github.com/hedronvision/bazel-compile-commands-extractor

## Building with Go

Ensure you have at least Go 1.17 installed.

`go build generate-compile-commands.go`

## Building with Bazel

`bazel build :generate_compile_commands`

## Glossary

 - [Compilation Database](https://clang.llvm.org/docs/JSONCompilationDatabase.html)
 - [Bazel](https://bazel.build/)
 - [Bazel Action Query](https://docs.bazel.build/versions/main/aquery.html)
