package bot

import (
	"regexp"
	"strings"
	"unicode"
)

// speakable is the part of text worth reading aloud. Links, @usernames and
// emoji would be spelled out ("at where air ends", "h t t p s") and charged
// for, so they are dropped. Line breaks are kept: they become pauses.
func speakable(text string) string {
	text = linkRe.ReplaceAllString(text, " ")
	text = usernameRe.ReplaceAllString(text, "$1")
	text = strings.Map(func(r rune) rune {
		if isEmojiPart(r) {
			return -1
		}
		return r
	}, text)
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

var (
	// linkRe matches web and Telegram links with or without a scheme.
	linkRe = regexp.MustCompile(`(?i)\b(?:https?://|www\.|t\.me/|telegram\.me/)\S+`)
	// usernameRe matches @usernames (4–32 characters), but not e-mail
	// addresses: the @ must start a word.
	usernameRe = regexp.MustCompile(`(^|[^\pL\pN_.])@[A-Za-z][A-Za-z0-9_]{3,31}\b`)
)

// isEmojiPart reports whether r is an emoji or one of the invisible pieces
// emoji are built from (variation selectors, joiners, skin tones, keycaps).
// Letters, digits and punctuation of every script are kept.
func isEmojiPart(r rune) bool {
	switch {
	case r == 0x200D, r == 0x20E3, r >= 0xFE00 && r <= 0xFE0F:
		return true
	case r >= 0x1F000 && r <= 0x1FAFF, r >= 0x2600 && r <= 0x27BF, r >= 0xE0000 && r <= 0xE007F:
		return true
	}
	return unicode.Is(unicode.So, r)
}

// hinglishWords are common Hindi words people type in English letters.
// English words that are also Hindi ("to", "me", "the") are left out.
var hinglishWords = map[string]bool{
	"hai": true, "hain": true, "hu": true, "hoon": true, "tha": true, "thi": true, "hoga": true, "hogi": true,
	"kya": true, "kyu": true, "kyun": true, "kaise": true, "kaisa": true, "kaha": true, "kahan": true, "kab": true, "kaun": true,
	"nahi": true, "nhi": true, "nahin": true, "haan": true, "acha": true, "accha": true, "achha": true, "thik": true, "theek": true,
	"abhi": true, "kal": true, "aaj": true, "jaldi": true, "baad": true, "phir": true, "fir": true,
	"bhut": true, "bahut": true, "bohot": true, "thoda": true, "zyada": true, "jyada": true, "sab": true, "kuch": true, "bas": true, "bss": true,
	"mai": true, "mein": true, "mujhe": true, "mera": true, "meri": true, "mere": true, "tum": true, "tu": true, "tera": true, "teri": true,
	"tumhara": true, "aap": true, "aapka": true, "hum": true, "hame": true, "humko": true, "wo": true, "woh": true, "yeh": true, "ye": true,
	"kar": true, "kr": true, "karo": true, "kro": true, "karna": true, "krna": true, "raha": true, "rha": true, "rahi": true, "rhi": true,
	"rahe": true, "rhe": true, "gaya": true, "gya": true, "diya": true, "liya": true, "dekh": true, "dekho": true, "bol": true, "bolo": true,
	"sun": true, "suno": true, "chal": true, "chalo": true, "pata": true, "matlab": true, "lekin": true, "aur": true, "bhi": true,
	"yaar": true, "yr": true, "bhai": true, "dost": true, "ji": true, "ka": true, "ki": true, "ke": true, "ko": true, "se": true,
}

// looksHinglish reports whether text is Hindi typed in English letters
// ("abhi to bhut time hai"). Devanagari text is real Hindi and isn't.
func looksHinglish(text string) bool {
	var words, hindi int
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) }) {
		for _, r := range w {
			if r > unicode.MaxLatin1 {
				return false // Devanagari or another script: not romanized
			}
		}
		words++
		if hinglishWords[w] {
			hindi++
		}
	}
	return hindi >= 2 && hindi*10 >= words*3 // at least 2 words and 30%
}
