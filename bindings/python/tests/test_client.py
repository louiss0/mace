import os
import tempfile
import unittest
from pathlib import Path

from mace_python import json


class ClientTest(unittest.TestCase):
    def test_passes_runtime_input_to_the_mace_cli(self) -> None:
        mace_path = os.environ.get("MACE_PATH")
        self.assertIsNotNone(mace_path, "set MACE_PATH to a built mace executable")

        with tempfile.TemporaryDirectory(prefix="mace-python-test-") as directory:
            path = Path(directory) / "runtime.mace"
            path.write_text(
                """|===|
schema Runtime: { env: string, };
|===|
[output = 'data', parse = Runtime]
{
  env: $env,
}
""",
                encoding="utf-8",
            )

            result = json(str(path), input='{ env: "prod", }', mace_path=mace_path)

        self.assertEqual({"env": "prod"}, result)


if __name__ == "__main__":
    unittest.main()
