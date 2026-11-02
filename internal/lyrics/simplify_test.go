package lyrics

import "testing"

// Traditional characters, in lines and their words, become simplified
// ones; Japanese, whose kanji are not Chinese's, is not taken for Chinese.
func TestSimplify(t *testing.T) {
	lines := Build("[00:01.00]富士山下 誰都只得那雙手\n[00:05.00]靠擁抱亦難任你擁有", "", "")
	if !Chinese(lines) {
		t.Fatal("Cantonese lyrics not taken for Chinese")
	}
	Simplify(lines)
	if lines[0].Text != "富士山下 谁都只得那双手" || lines[1].Text != "靠拥抱亦难任你拥有" {
		t.Errorf("simplified to %q, %q", lines[0].Text, lines[1].Text)
	}
	if w := lines[1].Words; len(w) == 0 || w[1].Text != "拥" {
		t.Errorf("words left as they were: %+v", w)
	}
	for _, text := range []string{"[00:01.00]夢ならばどれほどよかったでしょう", "[00:01.00]사랑해 永遠", "[00:01.00]Shape of you"} {
		if Chinese(Build(text, "", "")) {
			t.Errorf("%q taken for Chinese", text)
		}
	}
}
