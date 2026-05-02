import tempfile
import unittest
from pathlib import Path

from compress_mp4 import find_mp4_files


class FindMp4FilesTest(unittest.TestCase):
    def test_finds_mp4_files_recursively_case_insensitive(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            nested = root / "one" / "two" / "three"
            nested.mkdir(parents=True)

            target = nested / "clip.MP4"
            target.touch()
            (nested / "clip_output.MP4").touch()

            self.assertEqual(find_mp4_files(root), [target])


if __name__ == "__main__":
    unittest.main()
