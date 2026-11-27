package lyrics

import "testing"

// Phrase-level s2t converts Chinese lines, word tokens, and translations without regional word replacement.
func TestToTraditional(t *testing.T) {
	lines := Build("[00:01.00]皇后之后头发干燥", "[00:01.00]软件工程师在视频里默认登录", "")
	ToTraditional(lines)

	if got, want := lines[0].Text, "皇后之後頭髮乾燥"; got != want {
		t.Errorf("line text = %q, want %q", got, want)
	}
	wantWords := []string{"皇", "后", "之", "後", "頭", "髮", "乾", "燥"}
	if len(lines[0].Words) != len(wantWords) {
		t.Fatalf("words len = %d, want %d: %+v", len(lines[0].Words), len(wantWords), lines[0].Words)
	}
	for i, w := range wantWords {
		if lines[0].Words[i].Text != w {
			t.Errorf("word[%d] = %q, want %q", i, lines[0].Words[i].Text, w)
		}
	}
	if got, want := lines[0].Translation, "軟件工程師在視頻裏默認登錄"; got != want {
		t.Errorf("translation = %q, want %q", got, want)
	}

	// Word-timed YRC lyrics also disambiguate phrases across word tokens.
	yrcLines := Build("", "", "[1000,2000](1000,500,0)皇(1500,500,0)后(2000,500,0)之(2500,500,0)后")
	ToTraditional(yrcLines)
	if got, want := yrcLines[0].Text, "皇后之後"; got != want {
		t.Errorf("yrc text = %q, want %q", got, want)
	}
	for i, w := range []string{"皇", "后", "之", "後"} {
		if yrcLines[0].Words[i].Text != w {
			t.Errorf("yrc word[%d] = %q, want %q", i, yrcLines[0].Words[i].Text, w)
		}
	}

	// Foreign-language translations only convert Translation, leaving source lyrics untouched.
	foreign := Build("[00:01.00]桜の夜", "[00:01.00]皇后之后在视频里发现头发干燥", "")
	ToTraditionalTranslations(foreign)
	if foreign[0].Text != "桜の夜" {
		t.Errorf("foreign text modified: %q", foreign[0].Text)
	}
	if got, want := foreign[0].Translation, "皇后之後在視頻裏發現頭髮乾燥"; got != want {
		t.Errorf("foreign translation = %q, want %q", got, want)
	}

	// Credit lines remain recognized by IsCredit after ToTraditional conversion.
	withCredit := Build("[00:00.00]作词 : 林夕\n[00:05.00]皇后之后", "", "")
	ToTraditional(withCredit)
	if !IsCredit(withCredit[0].Text) {
		t.Errorf("credit line %q not recognized after ToTraditional", withCredit[0].Text)
	}
}
