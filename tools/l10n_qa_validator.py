#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
L10n QA Validator for Traditional Chinese (zh-TW / zh-Hant)
針對臺灣繁體中文在地化成果的自動化品質驗證引擎，支援直接交叉比對語系 JSON 目錄。
"""

import argparse
import json
from pathlib import Path
import re
import sys
from typing import Dict, List, Tuple


DEFAULT_SOURCE = Path(__file__).resolve().parent.parent / "internal" / "l10n" / "zh-Hans.json"
DEFAULT_TARGET = Path(__file__).resolve().parent.parent / "internal" / "l10n" / "zh-Hant.json"
DEFAULT_ST_CHARACTERS = Path(__file__).resolve().parent.parent / "internal" / "lyrics" / "STCharacters.txt"
DEFAULT_BLACKLIST = Path(__file__).resolve().parent / "zh_hant_blacklist.json"

HAN_PATTERN = r"[\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff]"


def load_strict_simplified_chars(path: Path = DEFAULT_ST_CHARACTERS) -> Dict[str, str]:
    mapping: Dict[str, str] = {}
    with path.open("r", encoding="utf-8") as f:
        for raw in f:
            line = raw.rstrip("\r\n")
            if not line or line.startswith("#") or "\t" not in line:
                continue
            k, v = line.split("\t", 1)
            candidates = v.split()
            if len(k) == 1 and candidates and k not in candidates:
                mapping[k] = candidates[0]
    return mapping


def load_blacklist(path: Path = DEFAULT_BLACKLIST) -> Dict[str, str]:
    with path.open("r", encoding="utf-8") as f:
        entries = json.load(f)
    blacklist = {item["term"]: item["suggestion"] for item in entries}
    if not blacklist:
        raise ValueError(f"Blacklist at {path} is empty")
    return blacklist


class L10nValidator:
    def __init__(
        self,
        custom_blacklist: Dict[str, str] = None,
        strict_simplified: Dict[str, str] = None,
    ):
        self.strict_simplified = (
            strict_simplified if strict_simplified is not None else load_strict_simplified_chars()
        )
        self.term_blacklist = custom_blacklist if custom_blacklist is not None else load_blacklist()

        self.printf_regex = re.compile(
            r"%(?:\[[0-9]+\])?[-+ #0]*(?:[0-9]+)?(?:\.[0-9]+)?(?:\[[0-9]+\])?[a-zA-Z]"
        )
        self.placeholder_regexes = [
            self.printf_regex.pattern,  # Go printf 格式: %s, %d, %w, %.1f, %+.1f, %[3]s
            r"\{[a-zA-Z0-9_]+\}",       # ICU 佔位符: {0}, {count}, {name}
        ]
        self.url_regex = re.compile(r"(?:https?|socks5?)://[^\s、，。；：！？）」』]+")
        self.han_regex = re.compile(HAN_PATTERN)
        self.half_width_parens_regex = re.compile(r"[()]")
        self.half_width_punct_regex = re.compile(
            rf"{HAN_PATTERN}[,?!;:]|[,?!;:]{HAN_PATTERN}"
            rf"|[a-zA-Z0-9][,?!;](?:\s|{HAN_PATTERN}|$)"
            r"|[a-zA-Z0-9]:(?:\s|$)"
        )
        self.pangu_regex = re.compile(rf"{HAN_PATTERN}[a-zA-Z0-9]|[a-zA-Z0-9]{HAN_PATTERN}")

    def extract_placeholders(self, text: str) -> List[str]:
        cleaned = text.replace("%%", "")
        extracted = []
        for reg in self.placeholder_regexes:
            matches = re.findall(reg, cleaned)
            extracted.extend(matches)
        return sorted(extracted)

    def _strip_for_text_checks(self, text: str) -> str:
        cleaned = text.replace("%%", "")
        for reg in self.placeholder_regexes:
            cleaned = re.sub(reg, "", cleaned)
        cleaned = self.url_regex.sub("", cleaned)
        return cleaned

    def validate_entry(self, source: str, target: str) -> List[Dict[str, str]]:
        issues = []

        if not target or not target.strip():
            issues.append({"level": "CRITICAL", "type": "EmptyTranslation", "msg": "翻譯內容為空。"})
            return issues

        simp_chars: List[Tuple[str, str]] = []
        seen_simp = set()
        for ch in target:
            if ch in self.strict_simplified and ch not in seen_simp:
                seen_simp.add(ch)
                simp_chars.append((ch, self.strict_simplified[ch]))

        stripped_source = self._strip_for_text_checks(source)
        if source.strip() == target.strip() and (
            (re.search(r"[a-zA-Z]{3,}", stripped_source) and "_" not in stripped_source)
            or bool(simp_chars)
        ):
            issues.append({
                "level": "ERROR" if simp_chars else "WARNING",
                "type": "Untranslated",
                "msg": "譯文與原文完全相同，疑似遺漏翻譯。",
            })

        if simp_chars:
            details = "、".join(f"「{ch}」→「{trad}」" for ch, trad in simp_chars)
            issues.append({
                "level": "ERROR",
                "type": "SimplifiedCharacter",
                "msg": f"偵測到殘留簡體字元：{details}。",
            })

        src_ph = self.extract_placeholders(source)
        tgt_ph = self.extract_placeholders(target)
        if src_ph != tgt_ph:
            missing = sorted(set(src_ph) - set(tgt_ph))
            extra = sorted(set(tgt_ph) - set(src_ph))
            detail = ""
            if missing:
                detail += f"遺漏: {missing} "
            if extra:
                detail += f"多餘: {extra} "
            issues.append({"level": "CRITICAL", "type": "PlaceholderMismatch", "msg": f"佔位符不匹配。{detail}".strip()})

        for term, suggestion in self.term_blacklist.items():
            if term in target:
                issues.append({
                    "level": "ERROR",
                    "type": "BlacklistedTerm",
                    "msg": f"偵測到非標準／違規術語「{term}」，建議替換為「{suggestion}」。",
                })

        if re.search(r"代理(?!伺服器)", target):
            issues.append({
                "level": "ERROR",
                "type": "BlacklistedTerm",
                "msg": "偵測到單獨使用的「代理」，依專案術語表應使用「代理伺服器」。",
            })

        if re.search(r"[\u201c\u201d\u2018\u2019]", target):
            issues.append({
                "level": "ERROR",
                "type": "InvalidQuotes",
                "msg": "檢測到簡體／彎引號 “” 或 ‘’，依臺灣規範應使用角標引號「」或『』。",
            })

        stripped_target = self._strip_for_text_checks(target)
        if self.han_regex.search(stripped_target) and (
            self.half_width_parens_regex.search(stripped_target)
            or self.half_width_punct_regex.search(stripped_target)
        ):
            issues.append({
                "level": "ERROR",
                "type": "HalfWidthPunctuation",
                "msg": "中文字串包含半形標點符號或半形括號，應使用全形標點與（）括號。",
            })

        if (
            "..." in target
            or "。。。" in target
            or any(len(m.group(0)) != 2 for m in re.finditer(r"\u2026+", target))
        ):
            issues.append({
                "level": "ERROR",
                "type": "InvalidEllipsis",
                "msg": "發現非標準省略號，應改用正體中文標準雙全形省略號「……」。",
            })

        if self.pangu_regex.search(stripped_target):
            issues.append({
                "level": "ERROR",
                "type": "PanguSpacingMissing",
                "msg": "中文字元與英文字母／半形數字之間缺少空格（盤古之白規範）。",
            })

        return issues

    def validate_catalogs(self, source: Dict[str, str], target: Dict[str, str]) -> List[Tuple[str, Dict[str, str]]]:
        findings: List[Tuple[str, Dict[str, str]]] = []
        for key, src_val in source.items():
            if key.startswith("@"):
                continue
            if key not in target:
                findings.append((key, {"level": "CRITICAL", "type": "MissingKey", "msg": "目標語系目錄遺漏此條目。"}))
                continue
            for issue in self.validate_entry(src_val, target[key]):
                findings.append((key, issue))

        for key in target:
            if key.startswith("@"):
                continue
            if key not in source:
                findings.append((key, {"level": "ERROR", "type": "ExtraKey", "msg": "目標語系目錄包含來源目錄不存在的條目。"}))

        return findings


def validate_files(source_path: Path, target_path: Path, validator: L10nValidator = None) -> List[Tuple[str, Dict[str, str]]]:
    validator = validator or L10nValidator()
    with source_path.open("r", encoding="utf-8") as f:
        source = json.load(f)
    with target_path.open("r", encoding="utf-8") as f:
        target = json.load(f)
    return validator.validate_catalogs(source, target)


def main(argv: List[str] = None) -> int:
    parser = argparse.ArgumentParser(description="Validate Traditional Chinese (zh-Hant) localization catalog.")
    parser.add_argument("--source", type=Path, default=DEFAULT_SOURCE, help="Path to source catalog JSON (zh-Hans.json)")
    parser.add_argument("--target", type=Path, default=DEFAULT_TARGET, help="Path to target catalog JSON (zh-Hant.json)")
    args = parser.parse_args(argv)

    findings = validate_files(args.source, args.target)
    if findings:
        for key, issue in findings:
            print(f"[{issue['level']}] {key} ({issue['type']}): {issue['msg']}")
        print(f"\nFound {len(findings)} issue(s) between {args.source} and {args.target}.", file=sys.stderr)
        return 1

    print(f"OK: {args.target} passed all L10n QA checks against {args.source}.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
