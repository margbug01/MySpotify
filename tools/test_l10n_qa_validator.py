import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from l10n_qa_validator import L10nValidator, main, validate_files


class TestL10nValidator(unittest.TestCase):
    def setUp(self):
        self.validator = L10nValidator()

    def test_real_catalogs_pass(self):
        root = Path(__file__).resolve().parent.parent / "internal" / "l10n"
        findings = validate_files(root / "zh-Hans.json", root / "zh-Hant.json", self.validator)
        self.assertEqual(findings, [])

    def test_rejects_violations(self):
        cases = [
            ("empty", "关闭", "", "EmptyTranslation"),
            ("untranslated", "Settings", "Settings", "Untranslated"),
            ("printf_mismatch", "已用 %.1f GB", "已使用 %d GB", "PlaceholderMismatch"),
            ("icu_mismatch", "{count} 首", "{n} 首", "PlaceholderMismatch"),
            ("blacklist", "默认", "默認", "BlacklistedTerm"),
            ("blacklist_cache", "缓存", "緩存空間", "BlacklistedTerm"),
            ("untranslated_simp_close", "关闭", "关闭", ("Untranslated", "SimplifiedCharacter")),
            ("untranslated_simp_default", "默认", "默认", ("Untranslated", "SimplifiedCharacter")),
            ("mixed_simp", "默认设置", "預設设置", "SimplifiedCharacter"),
            ("bare_proxy", "代理", "系統代理", "BlacklistedTerm"),
            ("curly_quotes", "删除", "刪除“歌單”", "InvalidQuotes"),
            ("half_punct", "你好，世界", "你好, 世界", "HalfWidthPunctuation"),
            ("half_colon_ascii", "Connect: 请重新登录", "Connect: 請重新登入", "HalfWidthPunctuation"),
            ("half_parens", "高音质 (256k)", "高音質 (256k)", "HalfWidthPunctuation"),
            ("ellipsis_ascii", "加载中…", "載入中...", "InvalidEllipsis"),
            ("ellipsis_lone", "加载中…", "載入中…", "InvalidEllipsis"),
            ("ellipsis_triple", "加载中…", "載入中………", "InvalidEllipsis"),
            ("pangu_missing", "打开 Spotify", "開啟Spotify", "PanguSpacingMissing"),
        ]
        for key, src, tgt, expected in cases:
            expected_types = (expected,) if isinstance(expected, str) else expected
            with self.subTest(key=key):
                issues = self.validator.validate_entry(src, tgt)
                types = [i["type"] for i in issues]
                for expected_type in expected_types:
                    self.assertIn(expected_type, types, f"{key}: got {issues}")

    def test_accepts_valid_strings_with_urls_brands_and_formats(self):
        cases = [
            ("brand", "已登录 Spotify", "已登入 Spotify"),
            ("url", "代理", "請使用 https://example.com:8443/v1?a=1,2 或 socks5://127.0.0.1:7890 代理伺服器"),
            ("printf", "歌词%s %.1f 秒（偏移 %+.1f 秒）", "歌詞%s %.1f 秒（偏移 %+.1f 秒）"),
            ("percent", "%d%%，装好会自己重新打开", "%d%%，安裝完成後會自動重新啟動"),
            ("icu", "{count} 首", "{count} 首"),
            ("units", "%.1f亿", "%.1f億"),
            ("full_parens_bitrate", "高音质 (256k)", "高音質（256k）"),
            ("full_parens_shortcut", "歌词提前 0.5 秒 ([)", "歌詞提前 0.5 秒（[）"),
            ("full_colon_ascii", "Connect: 请重新登录", "Connect：請重新登入"),
            ("identical_play", "播放", "播放"),
            ("identical_saved", "已收藏", "已收藏"),
        ]
        for key, src, tgt in cases:
            with self.subTest(key=key):
                self.assertEqual(self.validator.validate_entry(src, tgt), [])

    def test_cli_exit_codes(self):
        self.assertEqual(main([]), 0)
        with tempfile.TemporaryDirectory() as tmp:
            src = Path(tmp) / "src.json"
            tgt = Path(tmp) / "tgt.json"
            src.write_text(json.dumps({"@@locale": "zh-Hans", "默认": "默认"}), encoding="utf-8")
            tgt.write_text(json.dumps({"@@locale": "zh-Hant", "默认": "默認"}), encoding="utf-8")
            proc = subprocess.run(
                [sys.executable, str(Path(__file__).resolve().parent / "l10n_qa_validator.py"),
                 "--source", str(src), "--target", str(tgt)],
                capture_output=True, text=True,
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("BlacklistedTerm", proc.stdout)


if __name__ == "__main__":
    unittest.main()
