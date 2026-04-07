package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/louiss0/mace/codec"
	"github.com/louiss0/mace/internal/formatter"
	"github.com/sanity-io/litter"
	"github.com/spf13/cobra"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser"
	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
)

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	command := newRootCommand(stdout, stderr)
	command.SetArgs(args)

	if err := command.Execute(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	return 0
}

func newRootCommand(stdout io.Writer, stderr io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:           "mace",
		Short:         "Process Mace configuration files",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	command.SetOut(stdout)
	command.SetErr(stderr)
	command.AddCommand(newJSONCommand(), newImportCommand(), newNodesCommand(), newSourceCommand(), newLSPCommand())

	return command
}

func newJSONCommand() *cobra.Command {
	var injectionInput string

	command := &cobra.Command{
		Use:   "json <path>",
		Short: "Evaluate a Mace file and print JSON output",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			processorInstance := processor.New()
			if injectionInput != "" {
				injections, err := processor.ParseInjectionRecord(injectionInput)
				if err != nil {
					return err
				}
				processorInstance = processor.NewWithInjections(injections)
			}

			result, err := processorInstance.ProcessFile(args[0])
			if err != nil {
				return err
			}

			payload, err := json.MarshalIndent(outputValue(result.Output), "", "  ")
			if err != nil {
				return fmt.Errorf("marshal json output: %w", err)
			}

			_, err = fmt.Fprintln(command.OutOrStdout(), string(payload))
			return err
		},
	}

	command.Flags().StringVar(&injectionInput, "inject", "", "Mace record literal used for injectable values")

	return command
}

func newImportCommand() *cobra.Command {
	var inputFormat string
	var schemaOnly bool

	command := &cobra.Command{
		Use:   "import <path> [path...]",
		Short: "Convert JSON, YAML, or TOML into a Mace document",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !schemaOnly && len(args) != 1 {
				return fmt.Errorf("`mace import` accepts multiple paths only with --schema")
			}

			resolvedFormat, err := importFormat(args[0], inputFormat)
			if err != nil {
				return err
			}

			inputs := make([]string, 0, len(args))
			for _, path := range args {
				pathFormat, err := importFormat(path, inputFormat)
				if err != nil {
					return err
				}
				if pathFormat != resolvedFormat {
					return fmt.Errorf("all import paths must use the same format")
				}

				contents, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("read import file: %w", err)
				}
				inputs = append(inputs, string(contents))
			}

			source, err := importSource(resolvedFormat, inputs, schemaOnly)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(command.OutOrStdout(), source)
			return err
		},
	}

	command.Flags().StringVar(&inputFormat, "format", "", "Input format: json, yaml, or toml")
	command.Flags().BoolVar(&schemaOnly, "schema", false, "Infer a Mace schema document from the input shape")

	return command
}

func newNodesCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "nodes <path>",
		Short: "Parse a Mace file and print its node structure",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			file, err := parseFile(args[0])
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(command.OutOrStdout(), litter.Sdump(file))
			return err
		},
	}

	return command
}

func newSourceCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "source <path>",
		Short: "Parse a Mace file and print canonical Mace source",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			file, err := parseFile(args[0])
			if err != nil {
				return err
			}

			source, err := formatter.FormatFile(file)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(command.OutOrStdout(), source)
			return err
		},
	}

	return command
}

func outputValue(fields map[string]processor.Value) map[string]any {
	output := make(map[string]any, len(fields))
	for name, value := range fields {
		output[name] = valueToAny(value)
	}

	return output
}

func valueToAny(value processor.Value) any {
	switch value.Kind {
	case processor.ValueString:
		return value.String
	case processor.ValueInt:
		return value.Int
	case processor.ValueFloat:
		return value.Float
	case processor.ValueBoolean:
		return value.Boolean
	case processor.ValueArray:
		items := make([]any, 0, len(value.Array))
		for _, item := range value.Array {
			items = append(items, valueToAny(item))
		}
		return items
	case processor.ValueRecord:
		record := make(map[string]any, len(value.Record))
		for name, item := range value.Record {
			record[name] = valueToAny(item)
		}
		return record
	case processor.ValueUnknown:
		return nil
	default:
		return nil
	}
}

func importFormat(path string, inputFormat string) (string, error) {
	if inputFormat != "" {
		switch strings.ToLower(inputFormat) {
		case "json", "yaml", "toml":
			return strings.ToLower(inputFormat), nil
		default:
			return "", fmt.Errorf("unsupported import format %q", inputFormat)
		}
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "json", nil
	case ".yaml", ".yml":
		return "yaml", nil
	case ".toml":
		return "toml", nil
	default:
		return "", fmt.Errorf("cannot infer import format from %q; use --format", path)
	}
}

func importSource(inputFormat string, inputs []string, schemaOnly bool) (string, error) {
	if len(inputs) == 0 {
		return "", fmt.Errorf("missing import input")
	}

	switch inputFormat {
	case "json":
		if schemaOnly {
			return codec.ImportJSONSchemaSamples(inputs)
		}
		return codec.ImportJSON(inputs[0])
	case "yaml":
		if schemaOnly {
			return codec.ImportYAMLSchemaSamples(inputs)
		}
		return codec.ImportYAML(inputs[0])
	case "toml":
		if schemaOnly {
			return codec.ImportTOMLSchemaSamples(inputs)
		}
		return codec.ImportTOML(inputs[0])
	default:
		return "", fmt.Errorf("unsupported import format %q", inputFormat)
	}
}

func parseFile(path string) (ast.File, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return ast.File{}, fmt.Errorf("read mace file: %w", err)
	}

	tokens, err := lex(string(contents))
	if err != nil {
		return ast.File{}, err
	}

	file, err := parser.New(tokens).ParseFile()
	if err != nil {
		return ast.File{}, err
	}

	return file, nil
}

func lex(input string) ([]lexer.Token, error) {
	lexerInstance := lexer.New(input)
	tokens := []lexer.Token{}

	for {
		token, err := lexerInstance.NextToken()
		if err != nil {
			return nil, err
		}

		tokens = append(tokens, token)
		if token.Type == lexer.TokenEOF {
			return tokens, nil
		}
	}
}
