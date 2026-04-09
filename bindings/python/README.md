# mace-python

Official Python bindings for Mace.

## Status

This package currently wraps the `mace` CLI from this repository.
It provides a Python-native API while the language implementation remains in Go.

## Development

This package is managed with `uv`.

```bash
cd bindings/python
python -m uv sync
python -m uv build
```

## Usage

```python
from mace_python import json, import_json

output = json("./config.mace")
source = import_json('{"name": "Ada"}')
```

## API

- `json(path, inject=None, mace_path="mace", cwd=None)`
- `json_text(path, inject=None, mace_path="mace", cwd=None)`
- `source(path, mace_path="mace", cwd=None)`
- `nodes(path, mace_path="mace", cwd=None)`
- `import_json(input_text, mace_path="mace", cwd=None)`
- `import_yaml(input_text, mace_path="mace", cwd=None)`
- `import_toml(input_text, mace_path="mace", cwd=None)`
- `import_file(path, mace_path="mace", cwd=None)`
