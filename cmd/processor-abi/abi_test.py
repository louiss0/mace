"""Black-box checks of the exported processor ABI, not its Go implementation."""

import ctypes
import os
import tempfile
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from threading import Event, Thread


class ProcessorABITest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.native = ctypes.CDLL(os.environ["MACE_PROCESSOR_LIBRARY"])
        cls.native.mace_abi_major.restype = ctypes.c_uint32
        cls.native.mace_process_source.argtypes = [ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p]
        cls.native.mace_process_source.restype = ctypes.c_uint64
        cls.native.mace_process_file.argtypes = [ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p]
        cls.native.mace_process_file.restype = ctypes.c_uint64
        cls.native.mace_result_root.argtypes = [ctypes.c_uint64]
        cls.native.mace_result_root.restype = ctypes.c_uint64
        cls.native.mace_result_error.argtypes = [ctypes.c_uint64]
        cls.native.mace_result_error.restype = ctypes.c_void_p
        cls.native.mace_result_free.argtypes = [ctypes.c_uint64]
        cls.native.mace_value_kind.argtypes = [ctypes.c_uint64]
        cls.native.mace_value_kind.restype = ctypes.c_uint32
        cls.native.mace_value_record_length.argtypes = [ctypes.c_uint64]
        cls.native.mace_value_record_length.restype = ctypes.c_uint64
        cls.native.mace_value_record_key.argtypes = [ctypes.c_uint64, ctypes.c_uint64]
        cls.native.mace_value_record_key.restype = ctypes.c_void_p
        cls.native.mace_value_record_value.argtypes = [ctypes.c_uint64, ctypes.c_uint64]
        cls.native.mace_value_record_value.restype = ctypes.c_uint64
        cls.native.mace_value_string.argtypes = [ctypes.c_uint64]
        cls.native.mace_value_string.restype = ctypes.c_void_p
        cls.native.mace_value_int.argtypes = [ctypes.c_uint64]
        cls.native.mace_value_int.restype = ctypes.c_int64
        cls.native.mace_string_free.argtypes = [ctypes.c_void_p]
        cls.native.mace_request_new.argtypes = [ctypes.c_uint32]
        cls.native.mace_request_new.restype = ctypes.c_uint64
        cls.native.mace_request_free.argtypes = [ctypes.c_uint64]
        cls.native.mace_process_source_with_request.argtypes = [ctypes.c_uint64, ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p]
        cls.native.mace_process_source_with_request.restype = ctypes.c_uint64

    def read_string(self, pointer):
        try:
            return ctypes.string_at(pointer).decode("utf-8") if pointer else None
        finally:
            if pointer:
                self.native.mace_string_free(pointer)

    def test_source_produces_typed_record_without_cli(self):
        with tempfile.TemporaryDirectory() as directory:
            source = b"[output = 'data']\n{ name: \"Ada\", count: 42, }"
            result = self.native.mace_process_source(source, directory.encode(), None)
            self.assertEqual(1, self.native.mace_abi_major())
            try:
                self.assertIsNone(self.read_string(self.native.mace_result_error(result)))
                record = self.native.mace_result_root(result)
                self.assertEqual(9, self.native.mace_value_kind(record))
                self.assertEqual(2, self.native.mace_value_record_length(record))
                self.assertEqual("count", self.read_string(self.native.mace_value_record_key(record, 0)))
                count = self.native.mace_value_record_value(record, 0)
                self.assertEqual(3, self.native.mace_value_kind(count))
                self.assertEqual(42, self.native.mace_value_int(count))
            finally:
                self.native.mace_result_free(result)

    def test_entry_file_outside_workspace_is_rejected(self):
        with tempfile.TemporaryDirectory() as workspace, tempfile.TemporaryDirectory() as elsewhere:
            path = Path(elsewhere) / "config.mace"
            path.write_text("[output = 'data']\n{ name: 'Ada', }", encoding="utf-8")
            result = self.native.mace_process_file(str(path).encode(), workspace.encode(), None)
            try:
                self.assertIn("workspace", self.read_string(self.native.mace_result_error(result)))
            finally:
                self.native.mace_result_free(result)

    def test_symlinked_import_cannot_escape_workspace(self):
        with tempfile.TemporaryDirectory() as workspace, tempfile.TemporaryDirectory() as elsewhere:
            external = Path(elsewhere) / "types.mace"
            external.write_text("alias Age: int;", encoding="utf-8")
            try:
                (Path(workspace) / "types.mace").symlink_to(external)
            except OSError as error:
                self.skipTest(f"Symlink creation unavailable: {error}")
            path = Path(workspace) / "config.mace"
            path.write_text("|===|\nfrom './types.mace' import Age;\n|===|\n[output = 'data']\n{ age: 42, }", encoding="utf-8")
            result = self.native.mace_process_file(str(path).encode(), workspace.encode(), None)
            try:
                self.assertIn("escapes root", self.read_string(self.native.mace_result_error(result)))
            finally:
                self.native.mace_result_free(result)

    def test_deadline_stops_an_unresponsive_remote_import(self):
        entered = Event()
        release = Event()

        class SlowImport(BaseHTTPRequestHandler):
            def do_GET(self):
                entered.set()
                release.wait(5)
                self.send_response(200)
                self.end_headers()
                try:
                    self.wfile.write(b"alias Age: int;")
                except (BrokenPipeError, ConnectionAbortedError, ConnectionResetError):
                    pass

            def log_message(self, *_):
                pass

        server = ThreadingHTTPServer(("127.0.0.1", 0), SlowImport)
        thread = Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            with tempfile.TemporaryDirectory() as workspace:
                source = f"|===|\nfrom 'http://127.0.0.1:{server.server_port}/types.mace' import Age;\n|===|\n[output = 'data']\n{{ age: 42, }}"
                request = self.native.mace_request_new(150)
                try:
                    result = self.native.mace_process_source_with_request(request, source.encode(), workspace.encode(), None)
                    try:
                        self.assertTrue(entered.is_set())
                        self.assertIsNotNone(self.read_string(self.native.mace_result_error(result)))
                    finally:
                        self.native.mace_result_free(result)
                finally:
                    self.native.mace_request_free(request)
        finally:
            release.set()
            server.shutdown()
            server.server_close()
            thread.join(timeout=2)

    def test_file_diagnostics_do_not_require_stderr_parsing(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "broken.mace"
            path.write_text("not a valid Mace file", encoding="utf-8")
            result = self.native.mace_process_file(str(path).encode(), directory.encode(), None)
            try:
                self.assertIsNotNone(self.read_string(self.native.mace_result_error(result)))
                self.assertEqual(0, self.native.mace_result_root(result))
            finally:
                self.native.mace_result_free(result)


if __name__ == "__main__":
    unittest.main()
