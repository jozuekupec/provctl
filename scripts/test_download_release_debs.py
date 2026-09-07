import importlib.util
import pathlib
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location(
    "download", pathlib.Path(__file__).with_name("download-release-debs.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class DownloadTests(unittest.TestCase):
    def test_pagination(self):
        import json
        with mock.patch.object(module.subprocess, "check_output", side_effect=[
            json.dumps([{}] * 100), json.dumps([{}]),
        ]) as command:
            self.assertEqual(len(list(module.api_pages("repos/a/b/releases"))), 101)
            self.assertIn("page=2", command.call_args.args[0][-1])

    def test_routes_prereleases_and_ignores_drafts(self):
        releases = [{"id": 1, "draft": False, "prerelease": False},
                    {"id": 2, "draft": False, "prerelease": True},
                    {"id": 3, "draft": True, "prerelease": False}]
        assets = [{"id": 7, "name": "provctl_1_amd64.deb", "size": 3}]
        def run(args, stdout, check):
            stdout.write(b"deb")
        with tempfile.TemporaryDirectory() as directory:
            output = pathlib.Path(directory) / "incoming"
            with mock.patch.object(module, "api_pages", side_effect=[releases, assets, assets]), \
                    mock.patch.object(module.subprocess, "run", side_effect=run):
                module.download("owner/repo", output)
            self.assertEqual((output / "stable" / assets[0]["name"]).read_bytes(), b"deb")
            self.assertTrue((output / "testing" / assets[0]["name"]).exists())

    def test_rejects_traversal_before_download(self):
        releases = [{"id": 1, "draft": False, "prerelease": False}]
        assets = [{"id": 7, "name": "../bad.deb", "size": 3}]
        with tempfile.TemporaryDirectory() as directory:
            with mock.patch.object(module, "api_pages", side_effect=[releases, assets]), \
                    mock.patch.object(module.subprocess, "run") as command:
                with self.assertRaises(ValueError):
                    module.download("owner/repo", pathlib.Path(directory) / "incoming")
                command.assert_not_called()


if __name__ == "__main__":
    unittest.main()
