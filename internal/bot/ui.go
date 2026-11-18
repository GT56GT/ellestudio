package bot

// ui.go is the single home of Ellie Studio's look: icons, labels, the screen
// layout (header → body → buttons → nav row), and every screen that needs no
// API call. Every screen follows the same rules:
//
//   - header: "<icon> <b>Title</b>" and an optional italic subtitle;
//   - all dynamic text escaped;
//   - buttons are icon + 1–3 words, two per row (three in compact pickers);
//   - the last row navigates: "◀ <parent>" and "🏠 Home" (Home screen excepted);
//   - pickers mark the current value with ✅ and redraw in place.

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"ellestudio/internal/cartesia"
	"ellestudio/internal/telegram"
)

// ---------------------------------------------------------------- icons & labels

const (
	icStudio   = "🎙"
	icVoices   = "🎧"
	icStyle    = "🎛"
	icEmotion  = "🎭"
	icSpeed    = "⚡"
	icVolume   = "🔊"
	icLang     = "🌐"
	icModel    = "🧠"
	icOutput   = "📦"
	icListen   = "📝"
	icCreate   = "🧬"
	icDict     = "📖"
	icSettings = "⚙️"
	icHelp     = "❓"
	icPlay     = "▶️"
	icAgain    = "🔁"
	icHome     = "🏠"
	icBack     = "◀"
	icOK       = "✅"
	icWait     = "⏳"
	icWarn     = "⚠️"
	icCancel   = "✖"
	icLock     = "🔒"
	icLocalize = "🌍"
)

// Persistent menu labels (the reply keyboard under the input box). Tapping
// one sends its text, which handleMessage routes before anything else.
const (
	menuSpeak      = icStudio + " Speak"
	menuVoices     = icVoices + " Voices"
	menuStyle      = icStyle + " Style"
	menuTranscribe = icListen + " Transcribe"
	menuHome       = icHome + " Home"
	menuHelp       = icHelp + " Help"
)

func menuKeyboard() *telegram.ReplyKeyboardMarkup {
	b := func(s string) telegram.KeyboardButton { return telegram.KeyboardButton{Text: s} }
	return &telegram.ReplyKeyboardMarkup{
		Keyboard: [][]telegram.KeyboardButton{
			{b(menuSpeak), b(menuVoices), b(menuStyle)},
			{b(menuTranscribe), b(menuHome), b(menuHelp)},
		},
		ResizeKeyboard:        true,
		IsPersistent:          true,
		InputFieldPlaceholder: "Type text to hear it…",
	}
}

// ---------------------------------------------------------------- layout kit

// screen is one message: HTML text plus an inline keyboard.
type screen struct {
	text string
	kb   *telegram.InlineKeyboardMarkup
}

// header renders the title line and optional subtitle, then a blank line.
func header(icon, title, subtitle string) string {
	s := icon + " <b>" + html.EscapeString(title) + "</b>"
	if subtitle != "" {
		s += "\n<i>" + html.EscapeString(subtitle) + "</i>"
	}
	return s + "\n\n"
}

// field renders "Label: value" with the value in bold.
func field(icon, label, value string) string {
	return icon + " " + label + ": <b>" + html.EscapeString(value) + "</b>\n"
}

type keys struct {
	rows [][]telegram.InlineKeyboardButton
}

func btn(text, data string) telegram.InlineKeyboardButton {
	return telegram.InlineKeyboardButton{Text: text, CallbackData: data}
}

func (k *keys) row(bs ...telegram.InlineKeyboardButton) *keys {
	if len(bs) > 0 {
		k.rows = append(k.rows, bs)
	}
	return k
}

// grid lays buttons out n per row.
func (k *keys) grid(n int, bs []telegram.InlineKeyboardButton) *keys {
	for i := 0; i < len(bs); i += n {
		k.row(bs[i:min(i+n, len(bs))]...)
	}
	return k
}

// nav appends the navigation row: back to parent (if any) and Home.
func (k *keys) nav(parentLabel, parentData string) *keys {
	if parentData == "" || parentData == "nav:home" {
		return k.row(btn(icHome+" Home", "nav:home"))
	}
	return k.row(btn(icBack+" "+parentLabel, parentData), btn(icHome+" Home", "nav:home"))
}

func (k *keys) build() *telegram.InlineKeyboardMarkup {
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: k.rows}
}

// mark prefixes a label with ✅ when it is the current choice.
func mark(label string, current bool) string {
	if current {
		return icOK + " " + label
	}
	return label
}

// ---------------------------------------------------------------- value labels

func fmtFactor(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) + "×" }

func outputLabel(asFile bool) string {
	if asFile {
		return "MP3 files"
	}
	return "voice notes"
}

// languages offered as buttons (code, label). Others via /lang <code>.
var languages = [][2]string{
	{"en", "🇬🇧 English"}, {"es", "🇪🇸 Español"}, {"fr", "🇫🇷 Français"},
	{"de", "🇩🇪 Deutsch"}, {"pt", "🇧🇷 Português"}, {"it", "🇮🇹 Italiano"},
	{"hi", "🇮🇳 हिन्दी"}, {"ja", "🇯🇵 日本語"}, {"ko", "🇰🇷 한국어"},
	{"zh", "🇨🇳 中文"}, {"nl", "🇳🇱 Nederlands"}, {"pl", "🇵🇱 Polski"},
	{"ru", "🇷🇺 Русский"}, {"tr", "🇹🇷 Türkçe"}, {"sv", "🇸🇪 Svenska"},
}

func langLabel(code string) string {
	if code == "" {
		return "auto"
	}
	for _, l := range languages {
		if l[0] == code {
			_, name, _ := strings.Cut(l[1], " ")
			return name
		}
	}
	return code
}

func styleSummary(s Settings) string {
	parts := []string{orDefault(s.Emotion, "natural")}
	if s.Speed != 1 {
		parts = append(parts, fmtFactor(s.Speed)+" speed")
	}
	if s.Volume != 1 {
		parts = append(parts, fmtFactor(s.Volume)+" volume")
	}
	parts = append(parts, langLabel(s.Language))
	return strings.Join(parts, " · ")
}

// ---------------------------------------------------------------- static screens

func (b *Bot) homeScreen(uid int64) screen {
	s := b.store.Get(uid)
	text := header(icStudio, "Ellie Studio", "Your pocket voice studio") +
		field(icVoices, "Voice", orDefault(s.VoiceName, "not chosen yet")) +
		field(icStyle, "Style", styleSummary(s)) +
		field(icOutput, "Output", outputLabel(s.AsFile)) +
		"\n💬 <i>Send any text to hear it · 🎤 send audio to transcribe it</i>"
	k := (&keys{}).
		row(btn(icVoices+" Voices", "nav:voices"), btn(icStyle+" Style", "nav:style")).
		row(btn(icPlay+" Hear a sample", "do:sample"), btn(icListen+" Transcribe", "do:transcribe")).
		row(btn(icCreate+" Create", "nav:create"), btn(icDict+" Pronunciation", "nav:dicts")).
		row(btn(icSettings+" Studio", "nav:studio"), btn(icHelp+" Help", "nav:help"))
	return screen{text, k.build()}
}

func (b *Bot) speakPrompt(uid int64) screen {
	s := b.store.Get(uid)
	text := header(icStudio, "Speak", "") +
		"Type anything and I'll say it with <b>" + html.EscapeString(orDefault(s.VoiceName, "your voice")) + "</b>.\n" +
		"<i>Up to " + strconv.Itoa(b.cfg.MaxTTSChars) + " characters per message.</i>"
	k := (&keys{}).row(btn(icVoices+" Change voice", "nav:voices"), btn(icStyle+" Style", "nav:style")).nav("", "")
	return screen{text, k.build()}
}

func transcribePrompt() screen {
	text := header(icListen, "Transcribe", "") +
		"Send me a <b>voice note</b> or an <b>audio file</b> and I'll write it down.\n" +
		"<i>mp3, ogg, wav, m4a, flac and webm work · up to 20 MB</i>\n\n" +
		"Tip: set your language for the best accuracy."
	k := (&keys{}).row(btn(icLang+" Language", "nav:lang")).nav("", "")
	return screen{text, k.build()}
}

func (b *Bot) styleScreen(uid int64) screen {
	s := b.store.Get(uid)
	text := header(icStyle, "Style", "How your voice sounds") +
		field(icEmotion, "Emotion", orDefault(s.Emotion, "natural")) +
		field(icSpeed, "Speed", fmtFactor(s.Speed)) +
		field(icVolume, "Volume", fmtFactor(s.Volume)) +
		field(icLang, "Language", langLabel(s.Language)) +
		field(icModel, "Model", s.Model) +
		field(icOutput, "Output", outputLabel(s.AsFile))
	k := (&keys{}).
		row(btn(icEmotion+" Emotion", "nav:emotion"), btn(icSpeed+" Speed", "nav:speed")).
		row(btn(icVolume+" Volume", "nav:volume"), btn(icLang+" Language", "nav:lang")).
		row(btn(icModel+" Model", "nav:model"), btn(icOutput+" Output", "nav:output")).
		row(btn(icPlay+" Hear a sample", "do:sample")).
		nav("", "")
	return screen{text, k.build()}
}

// featuredEmotions are the first emotion screen; "More" lists all 58.
var featuredEmotions = []string{
	"neutral", "calm", "content", "happy",
	"excited", "curious", "sympathetic", "peaceful",
	"sad", "angry", "scared", "anxious",
	"sarcastic", "confident", "mysterious", "determined",
}

func (b *Bot) emotionScreen(uid int64, all bool) screen {
	cur := b.store.Get(uid).Emotion
	list, per, sub := featuredEmotions, 4, "Natural lets the model read the mood from your text"
	if all {
		list, per, sub = cartesia.Emotions, 3, "All 58 emotions"
	}
	var bs []telegram.InlineKeyboardButton
	for _, e := range list {
		bs = append(bs, btn(mark(e, e == cur), "e:"+e))
	}
	k := (&keys{}).grid(per, bs)
	if all {
		k.nav("Emotion", "nav:emotion")
	} else {
		k.row(btn(mark("🤖 Natural", cur == ""), "e:auto"), btn("➕ More emotions", "nav:emotions"))
		k.nav("Style", "nav:style")
	}
	text := header(icEmotion, "Emotion", sub) + "Now: <b>" + html.EscapeString(orDefault(cur, "natural")) + "</b>"
	return screen{text, k.build()}
}

var (
	speedPresets  = []float64{0.6, 0.8, 0.9, 1.0, 1.1, 1.2, 1.3, 1.5}
	volumePresets = []float64{0.5, 0.75, 1.0, 1.25, 1.5, 2.0}
)

func (b *Bot) rangeScreen(uid int64, volume bool) screen {
	s := b.store.Get(uid)
	icon, title, prefix, cmd, cur, presets := icSpeed, "Speed", "s:", "/speed 1.15", s.Speed, speedPresets
	lo, hi := cartesia.MinSpeed, cartesia.MaxSpeed
	if volume {
		icon, title, prefix, cmd, cur, presets = icVolume, "Volume", "vo:", "/volume 1.4", s.Volume, volumePresets
		lo, hi = cartesia.MinVolume, cartesia.MaxVolume
	}
	var bs []telegram.InlineKeyboardButton
	for _, p := range presets {
		bs = append(bs, btn(mark(fmtFactor(p), p == cur), prefix+strconv.FormatFloat(p, 'f', -1, 64)))
	}
	text := header(icon, title, fmt.Sprintf("From %s to %s · 1× is normal", fmtFactor(lo), fmtFactor(hi))) +
		"Now: <b>" + fmtFactor(cur) + "</b>\n<i>For an exact value, send " + cmd + "</i>"
	k := (&keys{}).grid(3, bs).nav("Style", "nav:style")
	return screen{text, k.build()}
}

func (b *Bot) outputScreen(uid int64) screen {
	asFile := b.store.Get(uid).AsFile
	text := header(icOutput, "Output", "How I send your audio") +
		"🎙 <b>Voice notes</b> play right in the chat.\n🎵 <b>MP3 files</b> are easy to save and share."
	k := (&keys{}).
		row(btn(mark("🎙 Voice notes", !asFile), "f:voice"), btn(mark("🎵 MP3 files", asFile), "f:file")).
		nav("Style", "nav:style")
	return screen{text, k.build()}
}

func (b *Bot) studioScreen(uid int64) screen {
	s := b.store.Get(uid)
	text := header(icSettings, "Studio", "Your settings and the service") +
		field(icVoices, "Voice", orDefault(s.VoiceName, "not chosen yet")) +
		field(icStyle, "Style", styleSummary(s)) +
		field(icModel, "Model", s.Model) +
		field(icDict, "Pronunciation", orDefault(s.DictName, "none")) +
		field(icOutput, "Output", outputLabel(s.AsFile)) +
		"\n" + b.allowanceLines(uid)
	k := (&keys{}).
		row(btn(icDict+" Pronunciation", "nav:dicts"), btn(icStyle+" Style", "nav:style")).
		row(btn("📊 Service status", "nav:status"), btn("📈 Usage", "nav:usage")).
		row(btn("🧹 Reset my settings", "nav:reset")).
		nav("", "")
	return screen{text, k.build()}
}

func resetScreen() screen {
	text := header("🧹", "Reset settings?", "") +
		"Your voice, style and pronunciation choices go back to the studio defaults."
	k := (&keys{}).row(btn("🧹 Yes, reset", "reset:yes"), btn(icCancel+" Keep them", "nav:studio")).nav("", "")
	return screen{text, k.build()}
}

func (b *Bot) createScreen(uid int64) screen {
	if !b.isAdmin(uid) {
		text := header(icCreate, "Create", "") + icLock + " Creating and changing voices is limited to studio admins."
		return screen{text, (&keys{}).nav("", "").build()}
	}
	s := b.store.Get(uid)
	text := header(icCreate, "Create", "Make voices of your own") +
		"🧬 <b>Clone</b> a voice from a short recording.\n" +
		"🌍 <b>Localize</b> " + html.EscapeString(orDefault(s.VoiceName, "your voice")) + " into another accent.\n" +
		"📖 <b>Pronunciation</b> teaches me how to say names and brands."
	k := (&keys{}).
		row(btn(icCreate+" Clone a voice", "do:clone"), btn(icLocalize+" Localize voice", "nav:localize")).
		row(btn(icDict+" New dictionary", "do:newdict")).
		nav("", "")
	return screen{text, k.build()}
}

// ---------------------------------------------------------------- help

var helpTopics = []struct{ id, label, title, body string }{
	{"speak", icStudio + " Speaking", "Speaking", `Send any text and I reply with a voice note.

Under every voice note:
🔁 <b>Again</b> makes it once more (try it after changing the style)
🎭 <b>Emotion</b>, 🎧 <b>Voice</b>, 🎛 <b>Style</b> change how it sounds

Tip: punctuation shapes delivery. Commas add pauses, "!" adds energy.

Everyone gets a daily allowance of speech and transcription. See what's left in ⚙️ Studio.`},
	{"voices", icVoices + " Voices", "Voices", `Open 🎧 <b>Voices</b> to browse the library.

• Tap a name to use that voice
• Tap 🔊 to hear a free preview first
• Filter by ♀ / ♂, or 🔍 search by name or mood
• "Mine" shows voices you created`},
	{"listen", icListen + " Transcribing", "Transcribing", `Send a voice note, audio file or video note and I'll write it down.

Set your language in 🎛 Style → 🌐 Language for the best accuracy.
Under each transcript, 🔊 <b>Read aloud</b> speaks it back in your voice.`},
	{"create", icCreate + " Cloning", "Cloning", `🧬 Create → Clone a voice:
1. Send a clean 5–15 second recording of one person
2. Give the voice a name

Use a quiet room and natural speech. Only clone voices you have permission to use.`},
	{"commands", "⌨️ Commands", "Commands", `Everything has a button, but if you like typing:

/menu home screen · /voices [search] · /style
/emotion [name] · /speed 1.1 · /volume 1.2 · /lang fr
/say text · /transcribe (reply to audio) · /clone name
/accents · /localize accent · /rename name
/dicts · /newdict · /detect · /status · /usage
/cancel stops whatever you're in the middle of`},
}

func helpScreen(topic string) screen {
	for _, t := range helpTopics {
		if t.id == topic {
			return screen{header(icHelp, t.title, "") + t.body, (&keys{}).nav("Help", "nav:help").build()}
		}
	}
	var bs []telegram.InlineKeyboardButton
	for _, t := range helpTopics {
		bs = append(bs, btn(t.label, "help:"+t.id))
	}
	text := header(icHelp, "Help", "Pick a topic") +
		"The quick version: <b>send text</b> to hear it, <b>send audio</b> to read it.\n" +
		"The menu under the message box takes you anywhere, and 🏠 Home always brings you back."
	return screen{text, (&keys{}).grid(2, bs).nav("", "").build()}
}

// ---------------------------------------------------------------- result keyboards

// afterSpeech sits under every generated voice note.
func afterSpeech() *telegram.InlineKeyboardMarkup {
	return (&keys{}).
		row(btn(icAgain+" Again", "do:again"), btn(icEmotion+" Emotion", "nav:emotion")).
		row(btn(icVoices+" Voice", "nav:voices"), btn(icStyle+" Style", "nav:style")).
		nav("", "").
		build()
}

// afterTranscript sits under every transcript.
func afterTranscript() *telegram.InlineKeyboardMarkup {
	return (&keys{}).row(btn(icVolume+" Read aloud", "do:readback"), btn(icHome+" Home", "nav:home")).build()
}

// recovery offers the screen that fixes a failure. retry is the callback that
// repeats the failed action ("" when there is none to offer).
func recovery(err error, retry string) *telegram.InlineKeyboardMarkup {
	k := &keys{}
	if ae, ok := cartesia.AsAPIError(err); ok {
		switch ae.ErrorCode {
		case cartesia.ErrVoiceNotFound:
			k.row(btn(icVoices+" Pick a voice", "nav:voices"))
		case cartesia.ErrVoiceModelMismatch:
			k.row(btn(icModel+" Model", "nav:model"), btn(icVoices+" Voice", "nav:voices"))
		case cartesia.ErrModelNotFound:
			k.row(btn(icModel+" Model", "nav:model"))
		case cartesia.ErrLanguageNotSupported:
			k.row(btn(icLang+" Language", "nav:lang"))
		case cartesia.ErrConcurrencyLimited:
			if retry != "" {
				k.row(btn(icAgain+" Try again", retry))
			}
		}
	}
	return k.nav("", "").build()
}
