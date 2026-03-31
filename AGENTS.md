# AGENTS

## Cobra Output

When implementing Cobra commands in this repository, do not thread `io.Writer`
parameters through subcommand constructor signatures just to print output.
Prefer Cobra's built-in `command.OutOrStdout()` and `command.ErrOrStderr()`
inside `RunE` handlers so command output stays aligned with Cobra's standard
writer configuration.
