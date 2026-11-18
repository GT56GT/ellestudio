package bot

// flows.go holds the guided, multi-step conversations (search, clone,
// dictionary, rename). A flow waits for the user's next message; any command,
// menu button, or ✖ Cancel ends it, and it expires on its own after a while.

import (
	"context"
	"html"
	"strings"
	"time"
	"unicode/utf8"

	"ellestudio/internal/telegram"
)

type flowKind int

const (
	flowSearch flowKind = iota + 1
	flowCloneClip
	flowCloneName
	flowDictName
	flowDictItems
	flowRename
)

const flowTTL = 15 * time.Minute

type flow struct {
	kind    flowKind
	clip    audioRef // clone: the clip from step 1
	name    string   // dictionary: name from step 1
	voiceID string   // rename: which voice
	expires time.Time
}

func (b *Bot) setFlow(uid int64, f *flow) {
	f.expires = time.Now().Add(flowTTL)
	b.sessMu.Lock()
	b.flows[uid] = f
	b.sessMu.Unlock()
}

func (b *Bot) getFlow(uid int64) *flow {
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	f := b.flows[uid]
	if f != nil && time.Now().After(f.expires) {
		delete(b.flows, uid)
		return nil
	}
	return f
}

// clearFlow ends any flow and reports whether one was running.
func (b *Bot) clearFlow(uid int64) bool {
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	_, had := b.flows[uid]
	delete(b.flows, uid)
	return had
}

func cancelKeys() *telegram.InlineKeyboardMarkup {
	return (&keys{}).row(btn(icCancel+" Cancel", "cancel")).build()
}

func cancelledScreen() screen {
	return screen{header(icCancel, "Cancelled", "") + "Nothing was changed.", (&keys{}).nav("", "").build()}
}

// ---------------------------------------------------------------- prompts

func searchPrompt() screen {
	return screen{
		header("🔍", "Search voices", "") + "Type a name, a mood or a style, like <i>warm</i>, <i>narrator</i> or <i>British</i>.",
		cancelKeys(),
	}
}

func clonePromptClip() screen {
	return screen{
		header(icCreate, "Clone a voice", "Step 1 of 2 · the recording") +
			"Send a <b>5–15 second</b> recording of one person speaking naturally.\n" +
			"🎤 A voice note works great. Find a quiet spot and skip the music.\n\n" +
			"<i>Only clone voices you have permission to use.</i>",
		cancelKeys(),
	}
}

func clonePromptName() screen {
	return screen{
		header(icCreate, "Clone a voice", "Step 2 of 2 · the name") +
			"Got the recording 👍\nWhat should this voice be called?",
		cancelKeys(),
	}
}

func dictPromptName() screen {
	return screen{
		header(icDict, "New pronunciation dictionary", "Step 1 of 2 · the name") +
			"What should this dictionary be called? For example <i>Brand names</i>.",
		cancelKeys(),
	}
}

func dictPromptItems(name, problem string) screen {
	text := header(icDict, "New pronunciation dictionary", "Step 2 of 2 · the words") +
		"Send the words for <b>" + html.EscapeString(name) + "</b>, one per line:\n\n" +
		"<code>Cartesia = car-TEE-zhuh\nEllie = EH-lee</code>"
	if problem != "" {
		text += "\n\n" + icWarn + " " + html.EscapeString(problem)
	}
	return screen{text, cancelKeys()}
}

func renamePrompt(current string) screen {
	return screen{
		header("✏️", "Rename voice", "") + "What should <b>" + html.EscapeString(current) + "</b> be called now?",
		cancelKeys(),
	}
}

// ---------------------------------------------------------------- starting flows

// startFlow begins a guided step and posts its first prompt.
func (b *Bot) startFlow(ctx context.Context, chatID, uid int64, kind flowKind) {
	switch kind {
	case flowSearch:
		b.setFlow(uid, &flow{kind: flowSearch})
		b.sendScreen(ctx, chatID, 0, searchPrompt())
	case flowCloneClip, flowDictName, flowRename:
		if !b.isAdmin(uid) {
			b.sendScreen(ctx, chatID, 0, b.createScreen(uid))
			return
		}
		switch kind {
		case flowCloneClip:
			b.setFlow(uid, &flow{kind: flowCloneClip})
			b.sendScreen(ctx, chatID, 0, clonePromptClip())
		case flowDictName:
			b.setFlow(uid, &flow{kind: flowDictName})
			b.sendScreen(ctx, chatID, 0, dictPromptName())
		case flowRename:
			s := b.store.Get(uid)
			if s.VoiceID == "" {
				b.sendScreen(ctx, chatID, 0, noVoiceScreen())
				return
			}
			b.setFlow(uid, &flow{kind: flowRename, voiceID: s.VoiceID})
			b.sendScreen(ctx, chatID, 0, renamePrompt(orDefault(s.VoiceName, "this voice")))
		}
	}
}

// ---------------------------------------------------------------- continuing flows

// continueFlow feeds the user's message into the flow they are in.
func (b *Bot) continueFlow(ctx context.Context, m *telegram.Message, f *flow, text string, a audioRef, hasAudio bool) {
	uid := m.From.ID
	if f.kind == flowCloneClip {
		if !hasAudio {
			b.replyHTML(ctx, m, icCreate+" This step needs a <b>recording</b>. Send a voice note or audio file, or tap Cancel.", cancelKeys())
			return
		}
		f.clip = a
		f.kind = flowCloneName
		b.setFlow(uid, f)
		s := clonePromptName()
		b.replyHTML(ctx, m, s.text, s.kb)
		return
	}
	if hasAudio || text == "" {
		b.replyHTML(ctx, m, "✍️ I'm waiting for you to <b>type</b> an answer. Or tap Cancel.", cancelKeys())
		return
	}

	switch f.kind {
	case flowSearch:
		b.clearFlow(uid)
		b.openVoices(ctx, m.Chat.ID, uid, text, nil)
	case flowCloneName:
		if utf8.RuneCountInString(text) > 100 {
			b.replyHTML(ctx, m, "That name is a bit long. Try something under 100 characters.", cancelKeys())
			return
		}
		b.clearFlow(uid)
		b.runClone(ctx, m, f.clip, text)
	case flowDictName:
		if strings.Contains(text, "=") || utf8.RuneCountInString(text) > 100 {
			b.replyHTML(ctx, m, "Just the dictionary's name for now, like <i>Brand names</i>.", cancelKeys())
			return
		}
		f.name, f.kind = text, flowDictItems
		b.setFlow(uid, f)
		s := dictPromptItems(text, "")
		b.replyHTML(ctx, m, s.text, s.kb)
	case flowDictItems:
		name, items, err := parseDict(f.name + "\n" + text)
		if err != nil {
			s := dictPromptItems(f.name, err.Error())
			b.replyHTML(ctx, m, s.text, s.kb)
			return
		}
		b.clearFlow(uid)
		b.createDict(ctx, m, name, items)
	case flowRename:
		b.clearFlow(uid)
		b.renameVoice(ctx, m, f.voiceID, text)
	}
}
