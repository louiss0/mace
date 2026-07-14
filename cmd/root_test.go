package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"

	"github.com/louiss0/mace/codec"
	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
)

var tAssert *assert.Assertions

func TestCLI(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Cmd Suite")
}

func writeTempFile(name string, contents string) string {
	tempDir, err := os.MkdirTemp("", "mace-cli-*")
	tAssert.NoError(err)
	path := filepath.Join(tempDir, name)
	err = os.WriteFile(path, []byte(contents), 0o600)
	tAssert.NoError(err)
	return path
}

func writeMaceFile(contents string) string {
	return writeTempFile("config.mace", contents)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, os.ErrClosed
}

var _ = Describe("CLI", func() {
	It("exits with the command result from the process entrypoint", func() {
		previousArgs := os.Args
		previousExit := exit
		defer func() {
			os.Args = previousArgs
			exit = previousExit
		}()

		var code int
		os.Args = []string{"mace", "--help"}
		exit = func(value int) {
			code = value
		}

		main()

		tAssert.Equal(0, code)
	})

	It("returns 1 for unknown commands", func() {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := run([]string{"unknown"}, &stdout, &stderr)

		tAssert.Equal(1, exitCode)
		tAssert.Equal("", stdout.String())
		tAssert.Contains(stderr.String(), "unknown command")
	})

	Describe("helpers", func() {
		It("returns the current working directory as the activation dir", func() {
			dir, err := os.Getwd()
			tAssert.NoError(err)
			tAssert.Equal(dir, activationDir())
		})

		It("falls back to a relative directory when the working directory cannot be read", func() {
			previous := getWorkingDir
			defer func() { getWorkingDir = previous }()

			getWorkingDir = func() (string, error) {
				return "", os.ErrNotExist
			}

			tAssert.Equal(".", activationDir())
		})

		It("parses mace files and reports read errors", func() {
			path := writeMaceFile(`|===|
int value = 1;
|===|
[output = "data"]
{ value: value, }`)

			file, err := parseFile(path)
			tAssert.NoError(err)
			tAssert.NotEmpty(file.Output.Directives)

			_, err = parseFile(filepath.Join("missing", "config.mace"))
			tAssert.Error(err)
		})

		It("reports lexer and parser errors", func() {
			_, err := lex("@")
			tAssert.ErrorContains(err, "unexpected character")

			path := writeTempFile("broken.mace", `[output = "data"]
{ name: }`)
			_, err = parseFile(path)
			tAssert.Error(err)

			lexPath := writeTempFile("lexer.mace", `@`)
			_, err = parseFile(lexPath)
			tAssert.Error(err)
		})

		It("creates JSON and import commands", func() {
			jsonCommand := newJSONCommand()
			tAssert.Equal("json <path>", jsonCommand.Use)
			tAssert.Contains(jsonCommand.Short, "JSON")
			importCommand := newImportCommand()
			tAssert.Equal("import <path> [path...]", importCommand.Use)
			tAssert.Contains(importCommand.Short, "Mace files")
		})

		It("classifies import formats and paths", func() {
			jsonPath := writeTempFile("config.json", `{}`)
			yamlPath := writeTempFile("config.yaml", `name: Ada`)
			ymlPath := writeTempFile("config.yml", `name: Ada`)
			tomlPath := writeTempFile("config.toml", `name = "Ada"`)
			missingPath := writeTempFile("config", `name: Ada`)
			outputDir, err := os.MkdirTemp("", "mace-output-*")
			tAssert.NoError(err)

			format, err := importFormat(jsonPath)
			tAssert.NoError(err)
			tAssert.Equal("json", format)
			format, err = importFormat(yamlPath)
			tAssert.NoError(err)
			tAssert.Equal("yaml", format)
			format, err = importFormat(ymlPath)
			tAssert.NoError(err)
			tAssert.Equal("yaml", format)
			format, err = importFormat(tomlPath)
			tAssert.NoError(err)
			tAssert.Equal("toml", format)
			_, err = importFormat(missingPath)
			tAssert.Error(err)
			_, err = importFormat("config.txt")
			tAssert.Error(err)

			path, err := importOutputPath(jsonPath, "")
			tAssert.NoError(err)
			tAssert.Equal(strings.TrimSuffix(jsonPath, ".json")+".mace", path)
			path, err = importOutputPath(jsonPath, outputDir)
			tAssert.NoError(err)
			tAssert.Equal(filepath.Join(outputDir, "config.mace"), path)
		})

		It("summarizes mixed import failures", func() {
			goodPath := writeTempFile("good.yaml", "name: Ada")
			badPath := writeTempFile("bad.txt", "nope")

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newImportCommand()
			command.SetOut(&stdout)
			command.SetErr(&stderr)
			command.SetArgs([]string{goodPath, badPath})

			err := command.Execute()
			tAssert.Error(err)
			tAssert.Contains(stdout.String(), "Generated 1 Mace file(s); 1 file(s) failed.")
		})

		It("returns write errors from the import summary", func() {
			badOne := writeTempFile("bad-one.txt", "nope")
			badTwo := writeTempFile("bad-two.txt", "nope")

			var stderr bytes.Buffer

			command := newImportCommand()
			command.SetOut(failingWriter{})
			command.SetErr(&stderr)
			command.SetArgs([]string{badOne, badTwo})

			err := command.Execute()
			tAssert.Error(err)
		})

		It("converts processor values into any values", func() {
			tAssert.Equal("text", valueToAny(processor.Value{Kind: processor.ValueString, String: "text"}))
			tAssert.Equal(int64(4), valueToAny(processor.Value{Kind: processor.ValueInt, Int: 4}))
			tAssert.Equal(1.5, valueToAny(processor.Value{Kind: processor.ValueFloat, Float: 1.5}))
			tAssert.Equal("0xFF", valueToAny(processor.Value{Kind: processor.ValueHexInt, Int: 255}))
			tAssert.Equal("0x2.8", valueToAny(processor.Value{Kind: processor.ValueHexFloat, Float: 2.5}))
			tAssert.Equal(true, valueToAny(processor.Value{Kind: processor.ValueBoolean, Boolean: true}))
			tAssert.Equal([]any{"a", int64(2)}, valueToAny(processor.Value{Kind: processor.ValueArray, Array: []processor.Value{{Kind: processor.ValueString, String: "a"}, {Kind: processor.ValueInt, Int: 2}}}))
			tAssert.Equal(map[string]any{"name": "Ada"}, valueToAny(processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"name": {Kind: processor.ValueString, String: "Ada"}}}))
			tAssert.Nil(valueToAny(processor.Value{Kind: processor.ValueUnknown}))
			tAssert.Nil(valueToAny(processor.Value{Kind: processor.ValueKind(999)}))
		})
	})

	Describe("json", func() {
		It("prints evaluated output as JSON", func() {
			path := writeMaceFile(`|===|
int base = 2 + 2;
|===|
[output = "data"]
{
  base: base,
  profile: { name: "Ada", active: true, },
}`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"json", path})

			err := command.Execute()
			tAssert.NoError(err)
			tAssert.Equal("", stderr.String())
			tAssert.JSONEq(`{
  "base": 4,
  "profile": {
    "active": true,
    "name": "Ada"
  }
}`, stdout.String())
		})

		It("prints hexadecimal outputs as JSON strings", func() {
			path := writeMaceFile(`|===|
hex_int mask = 0xFF;
hex_float ratio = 0x2.8;
hex_float whole = 0x2.0;
|===|
[output = "data"]
{
  mask: mask,
  ratio: ratio,
  whole: whole,
}`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"json", path})

			err := command.Execute()
			tAssert.NoError(err)
			tAssert.Equal("", stderr.String())
			tAssert.JSONEq(`{
  "mask": "0xFF",
  "ratio": "0x2.8",
  "whole": "0x2.0"
}`, stdout.String())
		})

		It("accepts parse input as a Mace record literal", func() {
			path := writeMaceFile(`|===|
schema Runtime: { env: string, };
|===|
[output = "data", parse = Runtime]
{
  ok: true,
}`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"json", path, "--input", `{ env: "prod", }`})

			err := command.Execute()
			tAssert.NoError(err)
			tAssert.Equal("", stderr.String())
			tAssert.JSONEq(`{
  "ok": true
}`, stdout.String())
		})

		It("fails when parse input is malformed", func() {
			path := writeMaceFile(`|===|
schema Runtime: { env: string, };
|===|
[output = "data", parse = Runtime]
{
  ok: true,
}`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"json", path, "--input", `{ env: }`})

			err := command.Execute()
			tAssert.Error(err)
			tAssert.Equal("", stdout.String())
			tAssert.Equal("", stderr.String())
		})

		It("fails when parse input is missing required fields", func() {
			path := writeMaceFile(`|===|
schema Runtime: { env: string, };
|===|
[output = "data", parse = Runtime]
{
  ok: true,
}`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := run([]string{"json", path}, &stdout, &stderr)

			tAssert.Equal(1, exitCode)
			tAssert.Equal("", stdout.String())
			tAssert.Contains(stderr.String(), `missing required field`)
		})
	})

	Describe("import", func() {
		It("writes a Mace output block next to a JSON data file", func() {
			path := writeTempFile("config.json", `{
  "name": "Ada",
  "enabled": true,
  "profile": {
    "level": 2
  }
}`)
			outputPath := strings.TrimSuffix(path, ".json") + ".mace"

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"import", path})

			err := command.Execute()
			tAssert.NoError(err)
			tAssert.Equal("", stderr.String())
			tAssert.Contains(stdout.String(), outputPath)

			contents, err := os.ReadFile(outputPath)
			tAssert.NoError(err)
			tAssert.Equal(`[output = "data"]
{
  enabled: true,
  name: "Ada",
  profile: {
    level: 2
  }
}`, string(contents))
		})

		It("writes a Mace output schema block for JSON schema files", func() {
			path := writeTempFile("profile.json", `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "name": { "type": "string" },
    "age": { "type": "integer" }
  },
  "required": ["name"]
}`)
			outputPath := strings.TrimSuffix(path, ".json") + ".mace"

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"import", path})

			err := command.Execute()
			tAssert.NoError(err)
			tAssert.Equal("", stderr.String())
			tAssert.Contains(stdout.String(), outputPath)

			contents, err := os.ReadFile(outputPath)
			tAssert.NoError(err)
			tAssert.Equal(`[output = "schema"]
{
  age?: int,
  name: string
}`, string(contents))
		})

		It("writes multiple input files based on their extensions", func() {
			jsonPath := writeTempFile("config.json", `{
  "name": "Ada"
}`)
			yamlPath := writeTempFile("config.yaml", `name: Bob`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"import", jsonPath, yamlPath})

			err := command.Execute()
			tAssert.NoError(err)
			tAssert.Equal("", stderr.String())
			tAssert.Contains(stdout.String(), strings.TrimSuffix(jsonPath, ".json")+".mace")
			tAssert.Contains(stdout.String(), strings.TrimSuffix(yamlPath, ".yaml")+".mace")
			tAssert.Contains(stdout.String(), "Generated 2 Mace file(s).")
		})

		It("writes generated files to --output-dir when requested", func() {
			path := writeTempFile("config.toml", `name = "Ada"`)
			outputDir, err := os.MkdirTemp("", "mace-import-output-*")
			tAssert.NoError(err)
			outputPath := filepath.Join(outputDir, "config.mace")

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"import", path, "--output-dir", outputDir})

			err = command.Execute()
			tAssert.NoError(err)
			tAssert.Equal("", stderr.String())
			tAssert.Contains(stdout.String(), outputPath)

			contents, err := os.ReadFile(outputPath)
			tAssert.NoError(err)
			tAssert.Equal(`[output = "data"]
{
  name: "Ada"
}`, string(contents))
		})

		It("continues importing other files when one file fails", func() {
			validPath := writeTempFile("valid.json", `{
  "name": "Ada"
}`)
			invalidPath := writeTempFile("invalid.json", `{
  "nickname": null
}`)
			outputPath := strings.TrimSuffix(validPath, ".json") + ".mace"

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := run([]string{"import", validPath, invalidPath}, &stdout, &stderr)
			tAssert.Equal(1, exitCode)
			tAssert.Contains(stdout.String(), outputPath)
			tAssert.Contains(stdout.String(), "Generated 1 Mace file(s); 1 file(s) failed.")
			tAssert.Contains(stderr.String(), invalidPath)

			contents, err := os.ReadFile(outputPath)
			tAssert.NoError(err)
			tAssert.Equal(`[output = "data"]
{
  name: "Ada"
}`, string(contents))
		})

		It("fails for files without an extension", func() {
			path := writeTempFile("config", `name: Ada`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := run([]string{"import", path}, &stdout, &stderr)
			tAssert.Equal(1, exitCode)
			tAssert.Equal("", stdout.String())
			tAssert.Contains(stderr.String(), "missing file extension")
		})

		It("fails when output-dir would overwrite another generated file", func() {
			firstDir, err := os.MkdirTemp("", "mace-import-first-*")
			tAssert.NoError(err)
			secondDir, err := os.MkdirTemp("", "mace-import-second-*")
			tAssert.NoError(err)
			outputDir, err := os.MkdirTemp("", "mace-import-output-*")
			tAssert.NoError(err)

			firstPath := filepath.Join(firstDir, "config.json")
			secondPath := filepath.Join(secondDir, "config.yaml")
			tAssert.NoError(os.WriteFile(firstPath, []byte(`{"name":"Ada"}`), 0o600))
			tAssert.NoError(os.WriteFile(secondPath, []byte("name: Bob"), 0o600))

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := run([]string{"import", firstPath, secondPath, "--output-dir", outputDir}, &stdout, &stderr)
			tAssert.Equal(1, exitCode)
			tAssert.Contains(stderr.String(), "would overwrite generated file")
		})

		It("propagates writer failures while reporting generated files", func() {
			path := writeTempFile("config.json", `{"name":"Ada"}`)

			var stderr bytes.Buffer
			command := newRootCommand(failingWriter{}, &stderr)
			command.SetArgs([]string{"import", path})

			err := command.Execute()
			tAssert.Error(err)
		})

		It("fails when the output directory cannot be created", func() {
			path := writeTempFile("config.json", `{"name":"Ada"}`)
			outputDir := writeTempFile("output-dir", "")

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := run([]string{"import", path, "--output-dir", outputDir}, &stdout, &stderr)
			tAssert.Equal(1, exitCode)
			tAssert.Contains(stderr.String(), "create output directory")
		})

		It("fails when importing would write to an existing directory", func() {
			path := writeTempFile("config.json", `{"name":"Ada"}`)
			outputDir, err := os.MkdirTemp("", "mace-import-output-*")
			tAssert.NoError(err)
			tAssert.NoError(os.Mkdir(filepath.Join(outputDir, "config.mace"), 0o755))

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := run([]string{"import", path, "--output-dir", outputDir}, &stdout, &stderr)
			tAssert.Equal(1, exitCode)
			tAssert.Contains(stderr.String(), "write mace file")
		})
	})

	Describe("check", func() {
		It("formats command errors with their message", func() {
			err := &commandError{message: "check failed"}

			tAssert.Equal("check failed", err.Error())
		})

		It("does not write output when no files could be checked", func() {
			var stdout bytes.Buffer

			err := writeCheckOutput(&stdout, nil)

			tAssert.NoError(err)
			tAssert.Equal("", stdout.String())
		})

		It("returns writer errors while formatting check output", func() {
			err := writeCheckOutput(failingWriter{}, []codec.FileCheckReport{{}})
			tAssert.Error(err)
		})

		It("returns unsupported import format errors", func() {
			previous := importFormatFn
			defer func() { importFormatFn = previous }()

			importFormatFn = func(string) (string, error) {
				return "bogus", nil
			}

			path := writeTempFile("config.yaml", "name: Ada")
			_, err := importSourceFromPath(path, "")
			tAssert.ErrorContains(err, "unsupported import format")
		})

		It("formats multiple check reports", func() {
			var stdout bytes.Buffer

			err := writeCheckOutput(&stdout, []codec.FileCheckReport{{}, {}})

			tAssert.NoError(err)
			tAssert.NotEmpty(stdout.String())
		})

		It("returns formatter errors while writing check output", func() {
			previous := formatCheckReportFn
			defer func() { formatCheckReportFn = previous }()

			formatCheckReportFn = func(codec.CheckReport) (string, error) {
				return "", errors.New("format failed")
			}

			err := writeCheckOutput(io.Discard, []codec.FileCheckReport{{}})
			tAssert.ErrorContains(err, "format failed")
		})

		It("prints a plain Mace compatibility report for a single file", func() {
			path := writeTempFile("config.json", `{
  "name": "Ada",
  "foo-bar": true
}`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := run([]string{"check", path}, &stdout, &stderr)
			tAssert.Equal(1, exitCode)
			tAssert.Equal("", stderr.String())
			tAssert.Equal(`{
  syntax: [],
  key_incompatibility: [{
      path: "$[\"foo-bar\"]",
      reason: "key is not a valid Mace identifier",
      format: "json",
      key: "foo-bar"
    }],
  type_incompatibility: [],
  structure_incompatibility: []
}
`, stdout.String())
		})

		It("prints aggregated reports for multiple files", func() {
			jsonPath := writeTempFile("config.json", `{
  "foo-bar": true
}`)
			tomlPath := writeTempFile("config.toml", "name = \"Ada\"\n")

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := run([]string{"check", jsonPath, tomlPath}, &stdout, &stderr)
			tAssert.Equal(1, exitCode)
			tAssert.Equal("", stderr.String())
			tAssert.Contains(stdout.String(), "files")
			tAssert.Contains(stdout.String(), strings.ReplaceAll(jsonPath, "\\", "\\\\"))
			tAssert.Contains(stdout.String(), strings.ReplaceAll(tomlPath, "\\", "\\\\"))
			tAssert.Contains(stdout.String(), `key: "foo-bar"`)
		})

		It("returns success when no incompatibilities are found", func() {
			path := writeTempFile("config.toml", "name = \"Ada\"\n")

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := run([]string{"check", path}, &stdout, &stderr)
			tAssert.Equal(0, exitCode)
			tAssert.Equal("", stderr.String())
			tAssert.Equal(`{
  syntax: [],
  key_incompatibility: [],
  type_incompatibility: [],
  structure_incompatibility: []
}
`, stdout.String())
		})

		It("continues checking other files when one file fails", func() {
			validPath := writeTempFile("config.toml", "name = \"Ada\"\n")
			missingPath := filepath.Join(filepath.Dir(validPath), "missing.json")

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := run([]string{"check", missingPath, validPath}, &stdout, &stderr)

			tAssert.Equal(2, exitCode)
			tAssert.Contains(stderr.String(), missingPath)
			tAssert.Contains(stderr.String(), "read check file")
			tAssert.Contains(stdout.String(), `key_incompatibility: []`)
		})

		It("returns failure when every file fails before reporting", func() {
			path := filepath.Join(GinkgoT().TempDir(), "missing.json")

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := run([]string{"check", path}, &stdout, &stderr)

			tAssert.Equal(2, exitCode)
			tAssert.Equal("", stdout.String())
			tAssert.Contains(stderr.String(), "read check file")
		})

		It("propagates writer failures while printing check output", func() {
			path := writeTempFile("config.toml", "name = \"Ada\"\n")

			var stderr bytes.Buffer
			command := newRootCommand(failingWriter{}, &stderr)
			command.SetArgs([]string{"check", path})

			err := command.Execute()
			tAssert.Error(err)
		})
	})

	Describe("nodes", func() {
		It("prints the parsed node structure", func() {
			path := writeMaceFile(`[output = "data"] { result: 1 + 2, }`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"nodes", path})

			err := command.Execute()
			tAssert.NoError(err)
			tAssert.Equal("", stderr.String())
			tAssert.Contains(stdout.String(), "ast.File")
			tAssert.Contains(stdout.String(), "Value: \"data\"")
			tAssert.Contains(stdout.String(), "InfixExpression")
		})

		It("prints nodes for files that fail semantic validation", func() {
			path := writeMaceFile(`|===|
Unknown value = 1;
|===|
[output = "data"]
{
  result: 1,
}`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"nodes", path})

			err := command.Execute()
			tAssert.NoError(err)
			tAssert.Equal("", stderr.String())
			tAssert.Contains(stdout.String(), "VariableDeclaration")
			tAssert.Contains(stdout.String(), "Name: \"value\"")
			tAssert.Contains(stdout.String(), "NamedType")
		})

		It("fails when the node file cannot be read", func() {
			path := filepath.Join(GinkgoT().TempDir(), "missing.mace")

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"nodes", path})

			err := command.Execute()
			tAssert.Error(err)
		})
	})

	Describe("output", func() {
		It("prints canonical Mace source from the parsed file", func() {
			path := writeMaceFile(`|===|
from "./base.mace" import User;
schema User: { name: string, age?: int, };
|===|
[output = "data", schema = User]
{ name: "Ada", age: 1 + 2 * 3, }`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"output", path})

			err := command.Execute()
			tAssert.NoError(err)
			tAssert.Equal("", stderr.String())
			tAssert.Equal(`|===============================|
from "./base.mace" import User;
schema User: {
  name: string,
  age?: int
}
|===============================|
[output = "data", schema = User]
{
  name: "Ada",
  age: 1 + 2 * 3
}
`, stdout.String())
		})

		It("fails when the output file cannot be read", func() {
			path := filepath.Join(GinkgoT().TempDir(), "missing.mace")

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"output", path})

			err := command.Execute()
			tAssert.Error(err)
		})

		It("fails when the output file is malformed", func() {
			path := writeTempFile("broken.mace", `[output = "data"]
{ name: }`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"output", path})

			err := command.Execute()
			tAssert.Error(err)
		})

		It("propagates formatter failures while printing output", func() {
			previous := formatMaceFile
			defer func() { formatMaceFile = previous }()

			formatMaceFile = func(ast.File) (string, error) {
				return "", os.ErrInvalid
			}

			path := writeMaceFile(`[output = "data"]
{ name: "Ada", }`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"output", path})

			err := command.Execute()
			tAssert.Error(err)
		})
	})

	Describe("check", func() {
		It("builds the check command and error type", func() {
			command := newCheckCommand()
			tAssert.Equal("check <path> [path...]", command.Use)
			tAssert.Contains(command.Short, "compatibility issues")
			tAssert.Equal("message", (&commandError{message: "message"}).Error())
			commandError := &commandError{code: 2}
			tAssert.Equal(2, commandError.code)
		})
	})

	Describe("lsp", func() {
		It("registers the language server command", func() {
			command := newRootCommand(&bytes.Buffer{}, &bytes.Buffer{})

			found := false
			for _, child := range command.Commands() {
				if child.Name() == "lsp" {
					found = true
				}
			}

			tAssert.True(found)
		})

		It("builds a language server with default settings", func() {
			command := newLSPCommand()
			tAssert.Equal("lsp", command.Use)
			tAssert.Contains(command.Short, "language server")
		})
	})
})
