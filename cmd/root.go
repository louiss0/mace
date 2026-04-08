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
	var outputDir string

	command := &cobra.Command{
		Use:   "import <path> [path...]",
		Short: "Convert JSON, YAML, or TOML files into Mace files",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			writtenPaths := make([]string, 0, len(args))
			targetsByPath := map[string]string{}
			failedPaths := 0
			for _, path := range args {
				source, err := importSourceFromPath(path)
				if err != nil {
					_, _ = fmt.Fprintf(command.ErrOrStderr(), "%s: %v\n", path, err)
					failedPaths++
					continue
				}

				targetPath, err := importOutputPath(path, outputDir)
				if err != nil {
					_, _ = fmt.Fprintf(command.ErrOrStderr(), "%s: %v\n", path, err)
					failedPaths++
					continue
				}
				if priorPath, exists := targetsByPath[targetPath]; exists {
					_, _ = fmt.Fprintf(command.ErrOrStderr(), "%s: would overwrite generated file for %s\n", path, priorPath)
					failedPaths++
					continue
				}

				if err := os.WriteFile(targetPath, []byte(source), 0o600); err != nil {
					_, _ = fmt.Fprintf(command.ErrOrStderr(), "%s: write mace file: %v\n", path, err)
					failedPaths++
					continue
				}

				targetsByPath[targetPath] = path
				writtenPaths = append(writtenPaths, targetPath)
			}

			for _, writtenPath := range writtenPaths {
				if _, err := fmt.Fprintln(command.OutOrStdout(), writtenPath); err != nil {
					return err
				}
			}

			if failedPaths > 0 {
				if len(args) > 1 {
					if _, err := fmt.Fprintf(command.OutOrStdout(), "Generated %d Mace file(s); %d file(s) failed.\n", len(writtenPaths), failedPaths); err != nil {
						return err
					}
				}
				return fmt.Errorf("%d file(s) failed", failedPaths)
			}

			_, err := fmt.Fprintf(command.OutOrStdout(), "Generated %d Mace file(s).\n", len(writtenPaths))
			return err
		},
	}

	command.Flags().StringVar(&outputDir, "output-dir", "", "Directory where generated .mace files are written")

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

func importFormat(path string) (string, error) {
	extension := strings.ToLower(filepath.Ext(path))
	if extension == "" {
		return "", fmt.Errorf("missing file extension for %q", path)
	}

	switch extension {
	case ".json":
		return "json", nil
	case ".yaml", ".yml":
		return "yaml", nil
	case ".toml":
		return "toml", nil
	default:
		return "", fmt.Errorf("unsupported import extension %q", extension)
	}
}

func importSourceFromPath(path string) (string, error) {
	inputFormat, err := importFormat(path)
	if err != nil {
		return "", err
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read import file: %w", err)
	}

	switch inputFormat {
	case "json":
		return codec.ImportJSON(string(contents))
	case "yaml":
		return codec.ImportYAML(string(contents))
	case "toml":
		return codec.ImportTOML(string(contents))
	default:
		return "", fmt.Errorf("unsupported import format %q", inputFormat)
	}
}

func importOutputPath(path string, outputDir string) (string, error) {
	baseName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + ".mace"
	if outputDir == "" {
		return filepath.Join(filepath.Dir(path), baseName), nil
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	return filepath.Join(outputDir, baseName), nil
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
