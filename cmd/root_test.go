package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
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

var _ = Describe("CLI", func() {
	Describe("json", func() {
		It("prints evaluated output as JSON", func() {
			path := writeMaceFile(`|===|
int base = 2 + 2;
|===|
[output = data]
{
  base: base;
  profile: { name: "Ada"; active: true; };
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

		It("accepts injectable values as a Mace record literal", func() {
			path := writeMaceFile(`|===|
injectable string env = "dev";
|===|
[output = data]
{
  env: env;
}`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"json", path, "--inject", `{ env: "prod"; }`})

			err := command.Execute()
			tAssert.NoError(err)
			tAssert.Equal("", stderr.String())
			tAssert.JSONEq(`{
  "env": "prod"
}`, stdout.String())
		})

		It("fails when an injectable has no runtime value or initializer", func() {
			path := writeMaceFile(`|===|
injectable string env;
|===|
[output = data]
{
  env: env;
}`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := run([]string{"json", path}, &stdout, &stderr)

			tAssert.Equal(1, exitCode)
			tAssert.Equal("", stdout.String())
			tAssert.Contains(stderr.String(), `injectable "env" requires a runtime value`)
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
			tAssert.Equal(`[output = data]
{
  enabled: true;
  name: "Ada";
  profile: {
    level: 2;
  };
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
			tAssert.Equal(`[output = schema]
{
  age?: int;
  name: string;
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
	})

	Describe("nodes", func() {
		It("prints the parsed node structure", func() {
			path := writeMaceFile(`[output = data] { result: 1 + 2; }`)

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
[output = data]
{
  result: 1;
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
	})

	Describe("source", func() {
		It("prints canonical Mace source from the parsed file", func() {
			path := writeMaceFile(`from "./base.mace" import User;
|===|
schema User: { name: string; age?: int; };
|===|
[output = data, schema = User]
{ name: "Ada"; age: 1 + 2 * 3; }`)

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			command := newRootCommand(&stdout, &stderr)
			command.SetArgs([]string{"source", path})

			err := command.Execute()
			tAssert.NoError(err)
			tAssert.Equal("", stderr.String())
			tAssert.Equal(`from "./base.mace" import User;

|===============|
schema User: {
  name: string;
  age?: int;
};
|===============|
[output = data, schema = User]
{
  name: "Ada";
  age: 1 + 2 * 3;
}
`, stdout.String())
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
	})
})
