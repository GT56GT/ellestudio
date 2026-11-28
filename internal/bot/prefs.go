package bot

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"ellestudio/internal/cartesia"
	"ellestudio/internal/telegram"
)

var localeRe = regexp.MustCompile(`^[a-zA-Z]{2,3}(-[a-zA-Z]{2,4})?$`)

// ---------------------------------------------------------------- welcome

// welcome greets the user, pins the menu, then shows Home — or, when they
// arrived through a share link (/start v_<id>), the shared voice.
func (b *Bot) welcome(ctx context.Context, m *telegram.Message, startArg string) {
	uid := m.From.ID
	sharedID, shared := parseVoiceRef(startArg)
	name := html.EscapeString(orDefault(m.From.FirstName, "there"))
	var text string
	if !b.store.Known(uid) {
		b.track(ctx, uid, evStart, startSource(startArg), 0)
		bonus := b.applyReferral(ctx, m, startArg) // before the user is first saved
		text = b.firstVisitText(name, bonus)
		if shared {
			text = "👋 <b>Hi " + name + ", I'm Ellie!</b>\nSomeone shared a voice with you. It's on the card below: " +
				"tap <b>✅ Use this voice</b>, then send me any text to hear it."
		}
		if _, err := b.store.Update(uid, func(*Settings) {}); err != nil {
			b.log.Warn("save settings", "err", err)
		}
	} else {
		text = "👋 <b>Welcome back, " + name + "!</b> Your menu is below."
	}
	if _, err := b.tg.SendMessage(ctx, telegram.SendMessageParams{
		ChatID: m.Chat.ID, Text: text, ParseMode: "HTML", ReplyMarkup: menuKeyboard(),
	}); err != nil {
		b.log.Warn("sendMessage failed", "err", err)
	}
	if shared {
		b.sendScreen(ctx, m.Chat.ID, 0, b.voiceCard(ctx, uid, sharedID))
		return
	}
	if deepLinks[startArg] { // a "Continue in private chat" link from a group
		b.openDeepLink(ctx, m, startArg)
		return
	}
	b.sendScreen(ctx, m.Chat.ID, 0, b.homeScreen(uid))
}

func privateScreen(uid int64) screen {
	return screen{header(icLock, "Ellie Studio is private", "") +
		"Ask the studio owner to add your Telegram ID:\n<code>" + strconv.FormatInt(uid, 10) + "</code>", nil}
}

// ---------------------------------------------------------------- commands

// command runs a slash command. Every command lands on a screen.
func (b *Bot) command(ctx context.Context, m *telegram.Message, cmd, args string) {
	uid, chat := m.From.ID, m.Chat.ID
	send := func(s screen) { b.replyHTML(ctx, m, s.text, s.kb) }

	switch cmd {
	case "start":
		b.welcome(ctx, m, args)
	case "menu", "home":
		send(b.homeScreen(uid))
	case "help":
		send(helpScreen(args))
	case "say":
		if args == "" {
			send(b.speakPrompt(uid))
			return
		}
		b.speak(ctx, m, args)
	case "voices":
		b.openVoices(ctx, chat, uid, args, nil)
	case "myvoices":
		owned := true
		b.openVoices(ctx, chat, uid, args, &owned)
	case "voice":
		if id, ok := parseVoiceRef(args); ok {
			args = id
		}
		if args != "" {
			v, err := b.selectVoice(ctx, uid, args)
			if err != nil {
				b.fail(ctx, m, "select voice", err)
				return
			}
			send(voiceChosenScreen(v))
			return
		}
		send(b.voiceCard(ctx, uid, ""))
	case "preview":
		id := orDefault(args, b.store.Get(uid).VoiceID)
		if id == "" {
			send(noVoiceScreen())
			return
		}
		b.sendPreview(ctx, uid, chat, m.MessageID, id)
	case "style":
		send(b.styleScreen(uid))
	case "settings", "studio":
		send(b.studioScreen(uid))
	case "emotion":
		switch a := strings.ToLower(args); {
		case a == "":
			send(b.emotionScreen(uid, false))
		case a == "list" || a == "all":
			send(b.emotionScreen(uid, true))
		default:
			if err := b.setEmotion(uid, a); err != nil {
				send(withWarning(b.emotionScreen(uid, false), err))
				return
			}
			send(b.emotionScreen(uid, !contains(featuredEmotions, a) && a != "auto"))
		}
	case "speed", "volume":
		vol := cmd == "volume"
		if args != "" {
			if err := b.setFactor(uid, vol, args); err != nil {
				send(withWarning(b.rangeScreen(uid, vol), err))
				return
			}
		}
		send(b.rangeScreen(uid, vol))
	case "model":
		if args != "" {
			if err := b.setModel(ctx, uid, args); err != nil {
				send(withWarning(b.modelScreen(ctx, uid), err))
				return
			}
		}
		send(b.modelScreen(ctx, uid))
	case "lang", "language":
		if args != "" {
			if err := b.setLang(uid, args); err != nil {
				send(withWarning(b.langScreen(ctx, uid), err))
				return
			}
		}
		send(b.langScreen(ctx, uid))
	case "format", "output":
		switch strings.ToLower(args) {
		case "voice", "note":
			b.setOutput(uid, false)
		case "file", "audio", "mp3":
			b.setOutput(uid, true)
		}
		send(b.outputScreen(uid))
	case "accents":
		b.cmdAccents(ctx, m, args)
	case "localize":
		if args == "" {
			send(b.localizeScreen(ctx, uid))
			return
		}
		accent, _, _ := strings.Cut(args, " ")
		if target := b.sendScreen(ctx, chat, m.MessageID, screen{icWait + " Starting…", nil}); target != nil {
			b.runLocalize(ctx, uid, target, accent)
		}
	case "rename":
		if args == "" {
			b.startFlow(ctx, chat, uid, flowRename)
			return
		}
		b.renameVoice(ctx, m, b.store.Get(uid).VoiceID, args)
	case "deletevoice":
		send(b.deleteConfirmScreen(ctx, uid, orDefault(args, b.store.Get(uid).VoiceID)))
	case "dicts":
		send(b.dictsScreen(ctx, uid))
	case "newdict":
		if args == "" {
			b.startFlow(ctx, chat, uid, flowDictName)
			return
		}
		name, items, err := parseDict(args)
		if err != nil {
			send(screen{header(icDict, "New dictionary", "") + icWarn + " " + html.EscapeString(err.Error()) + "\n\nOr tap below and I'll guide you.",
				(&keys{}).row(btn(icDict+" New dictionary", "do:newdict")).nav("", "").build()})
			return
		}
		b.createDict(ctx, m, name, items)
	case "clone", "transcribe", "detect":
		if a, ok := audioOf(m.ReplyToMessage); ok {
			b.audioCommand(ctx, m, cmd, a, args)
			return
		}
		switch cmd {
		case "clone":
			b.startFlow(ctx, chat, uid, flowCloneClip)
		case "transcribe":
			send(transcribePrompt())
		case "detect":
			send(screen{header("🔏", "Watermark check", "") +
				"Reply to an audio message with /detect, or send audio with /detect as its caption.\n" +
				"<i>Audio must be 5 seconds to 5 minutes long. Needs a paid Cartesia plan.</i>", (&keys{}).nav("", "").build()})
		}
	case "reset":
		send(resetScreen())
	case "status":
		send(b.statusScreen(ctx))
	case "usage":
		send(b.usageScreen(ctx, uid))
	case "profile", "me":
		send(b.profileScreen(uid))
	case "invite":
		send(b.inviteScreen(uid))
	case "admin", "stats":
		if !b.isOwner(uid) {
			b.command(ctx, m, "unknown", "")
			return
		}
		if cmd == "stats" {
			send(b.statsScreen(ctx))
			return
		}
		send(b.adminScreen(ctx))
	case "groups", "blockgroup", "unblockgroup":
		if !b.isOwner(uid) {
			b.command(ctx, m, "unknown", "")
			return
		}
		if cmd == "groups" {
			send(b.groupsScreen(ctx))
			return
		}
		b.cmdBlockGroup(ctx, m, cmd == "blockgroup", args)
	default:
		send(screen{header("🤔", "Unknown command", "") + "Tap Help to see what I can do, or just send me some text.",
			(&keys{}).row(btn(icHelp+" Help", "nav:help")).nav("", "").build()})
	}
}

// ---------------------------------------------------------------- setters

func (b *Bot) setEmotion(uid int64, e string) error {
	switch e {
	case "auto", "none", "off", "natural":
		e = ""
	default:
		if !cartesia.IsEmotion(e) {
			return fmt.Errorf("I don't know the emotion %q. Pick one below.", e)
		}
	}
	_, err := b.store.Update(uid, func(s *Settings) { s.Emotion = e })
	return err
}

func (b *Bot) setFactor(uid int64, volume bool, raw string) error {
	lo, hi := cartesia.MinSpeed, cartesia.MaxSpeed
	if volume {
		lo, hi = cartesia.MinVolume, cartesia.MaxVolume
	}
	v, err := parseFactor(raw, lo, hi)
	if err != nil {
		return err
	}
	_, err = b.store.Update(uid, func(s *Settings) {
		if volume {
			s.Volume = v
		} else {
			s.Speed = v
		}
	})
	return err
}

func parseFactor(s string, lo, hi float64) (float64, error) {
	s = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(strings.ToLower(s)), "x"), "×")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("Send a number between %s and %s, like 1.1.", fmtFactor(lo), fmtFactor(hi))
	}
	if v < lo || v > hi {
		return 0, fmt.Errorf("That's out of range. Pick something from %s to %s.", fmtFactor(lo), fmtFactor(hi))
	}
	return v, nil
}

func (b *Bot) setModel(ctx context.Context, uid int64, id string) error {
	if !contains(b.ttsModels(ctx), id) {
		return fmt.Errorf("I don't know the model %q. Pick one below.", id)
	}
	_, err := b.store.Update(uid, func(s *Settings) { s.Model = id })
	return err
}

func (b *Bot) setLang(uid int64, raw string) error {
	lang := ""
	if l := strings.ToLower(strings.TrimSpace(raw)); l != "auto" && l != "off" && l != "none" {
		if !localeRe.MatchString(raw) {
			return fmt.Errorf("That doesn't look like a language code. Try en, fr, ja or en-GB.")
		}
		lang = normalizeLocale(raw)
	}
	_, err := b.store.Update(uid, func(s *Settings) { s.Language = lang })
	return err
}

func (b *Bot) setOutput(uid int64, asFile bool) {
	if _, err := b.store.Update(uid, func(s *Settings) { s.AsFile = asFile }); err != nil {
		b.log.Warn("save settings", "err", err)
	}
}

// normalizeLocale turns "en-gb" into "en-GB" and "FR" into "fr".
func normalizeLocale(s string) string {
	lang, region, ok := strings.Cut(strings.TrimSpace(s), "-")
	lang = strings.ToLower(lang)
	if !ok {
		return lang
	}
	if len(region) == 2 {
		region = strings.ToUpper(region)
	}
	return lang + "-" + region
}

// ---------------------------------------------------------------- API-backed screens

// ttsModels asks GET /models, falling back to the spec's enum.
func (b *Bot) ttsModels(ctx context.Context) []string {
	list, err := b.ct.ListModels(ctx, "tts")
	if err != nil || len(list) == 0 {
		return cartesia.TTSModels
	}
	ids := make([]string, 0, len(list))
	for _, m := range list {
		if len("m:"+m.ID) <= 64 {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

func (b *Bot) modelScreen(ctx context.Context, uid int64) screen {
	cur := b.store.Get(uid).Model
	var bs []telegram.InlineKeyboardButton
	for _, id := range b.ttsModels(ctx) {
		bs = append(bs, btn(mark(id, id == cur), "m:"+id))
	}
	text := header(icModel, "Model", "The Sonic engine that speaks") +
		"Now: <b>" + html.EscapeString(cur) + "</b>\n<i>The newest model usually sounds best.</i>"
	return screen{text, (&keys{}).grid(2, bs).nav("Style", "nav:style").build()}
}

func (b *Bot) langScreen(ctx context.Context, uid int64) screen {
	s := b.store.Get(uid)
	supported := map[string]bool{}
	if list, err := b.ct.ListModels(ctx, "tts"); err == nil {
		for _, md := range list {
			if md.ID == s.Model {
				for _, l := range md.SupportedLanguages {
					supported[baseLang(l)] = true
				}
			}
		}
	}
	var bs []telegram.InlineKeyboardButton
	for _, l := range languages {
		if len(supported) == 0 || supported[l[0]] {
			bs = append(bs, btn(mark(l[1], s.Language == l[0]), "l:"+l[0]))
		}
	}
	text := header(icLang, "Language", "Used for speaking, transcribing and cloning") +
		"Now: <b>" + html.EscapeString(langLabel(s.Language)) + "</b>\n" +
		"<i>Auto works for most text. For other languages send /lang code, like /lang en-GB.</i>"
	k := (&keys{}).row(btn(mark("🤖 Auto", s.Language == ""), "l:auto")).grid(3, bs).nav("Style", "nav:style")
	return screen{text, k.build()}
}

// dictsShown is how many dictionaries the picker lists (newest first).
const dictsShown = 20

func (b *Bot) dictsScreen(ctx context.Context, uid int64) screen {
	dicts, err := b.listDicts(ctx, uid)
	if err != nil {
		return errorScreen(err)
	}
	cur := b.store.Get(uid).DictID
	text := header(icDict, "Pronunciation", "Teach me how to say names and brands") +
		"Now: <b>" + html.EscapeString(orDefault(b.store.Get(uid).DictName, "none")) + "</b>"
	if len(dicts) == 0 {
		text += "\n\nYou don't have any dictionaries yet."
	}
	k := &keys{}
	for _, d := range dicts {
		if len("d:"+d.ID) <= 64 {
			k.row(btn(mark(fmt.Sprintf("%s · %d words", truncate(d.Name, 28), len(d.Items)), d.ID == cur), "d:"+d.ID))
		}
	}
	k.row(btn(mark("🚫 No dictionary", cur == ""), "d:-"))
	k.row(btn(icDict+" New dictionary", "do:newdict"))
	k.nav("", "")
	return screen{text, k.build()}
}

func (b *Bot) createDict(ctx context.Context, m *telegram.Message, name string, items []cartesia.PronunciationItem) {
	uid := m.From.ID
	if !b.isAdmin(uid) {
		ok, used, err := b.store.SpendDict(uid, today(), b.lim().DailyDicts)
		if err != nil {
			b.log.Warn("save usage", "err", err)
		}
		if !ok {
			s := noticeScreen("🎟", "Daily dictionary limit reached", fmt.Sprintf(
				"You've created <b>%d</b> of your <b>%d</b> dictionaries for today. You can make more in <b>%s</b> (midnight UTC).",
				used, b.lim().DailyDicts, fmtWait(untilReset())))
			b.replyHTML(ctx, m, s.text, s.kb)
			return
		}
	}
	d, err := b.ct.CreatePronunciationDict(ctx, cartesia.PronunciationDictRequest{
		Name: orDefault(stripTags(name), "My words"), Description: "Created in Ellie Studio", Items: items,
	})
	if err == nil {
		if cerr := b.claimOwner(ctx, kindDict, d.ID, uid); cerr != nil {
			// An unowned dictionary could never be picked again; undo it.
			dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			if derr := b.ct.DeletePronunciationDict(dctx, d.ID); derr != nil {
				b.log.Error("orphaned dictionary: created but could not record its owner or delete it", "dict", d.ID, "user", uid, "err", derr)
			}
			cancel()
			err = cerr
		}
	}
	if err != nil {
		if !b.isAdmin(uid) {
			b.store.RefundDict(uid, today())
		}
		b.fail(ctx, m, "create dict", err)
		return
	}
	if _, err := b.store.Update(m.From.ID, func(s *Settings) { s.DictID, s.DictName = d.ID, d.Name }); err != nil {
		b.log.Warn("save settings", "err", err)
	}
	text := header(icDict, d.Name+" is on", fmt.Sprintf("%d words", len(d.Items))) + "Send some text with those words to hear the difference."
	k := (&keys{}).row(btn(icDict+" Pronunciation", "nav:dicts")).nav("", "")
	b.replyHTML(ctx, m, text, k.build())
}

// listDicts returns the dictionaries uid may pick: their own, newest first,
// from the ownership registry. Admins see the account's dictionaries.
func (b *Bot) listDicts(ctx context.Context, uid int64) ([]cartesia.PronunciationDict, error) {
	if b.isAdmin(uid) {
		page, err := b.ct.ListPronunciationDicts(ctx, dictsShown, "")
		if err != nil {
			return nil, err
		}
		return page.Data, nil
	}
	items, err := b.owners.List(ctx, kindDict, uid, Cursor{}, dictsShown)
	if err != nil {
		return nil, b.storageErr(err)
	}
	dicts := make([]*cartesia.PronunciationDict, len(items))
	errs := make([]error, len(items))
	var wg sync.WaitGroup
	for i, it := range items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dicts[i], errs[i] = b.ct.GetPronunciationDict(ctx, it.ID)
		}()
	}
	wg.Wait()
	var out []cartesia.PronunciationDict
	for i, d := range dicts {
		if err := errs[i]; err != nil {
			if ae, ok := cartesia.AsAPIError(err); ok && ae.StatusCode == 404 {
				b.releaseOwner(ctx, kindDict, items[i].ID)
				continue
			}
			return nil, err
		}
		out = append(out, *d)
	}
	return out, nil
}

// parseDict reads "name\ntext = pronunciation\n..." (also accepts "=>" or ":").
func parseDict(args string) (string, []cartesia.PronunciationItem, error) {
	lines := strings.Split(strings.TrimSpace(args), "\n")
	name := strings.TrimSpace(lines[0])
	if name == "" || strings.Contains(name, "=") {
		return "", nil, fmt.Errorf("Start with a dictionary name on the first line.")
	}
	var items []cartesia.PronunciationItem
	for i, l := range lines[1:] {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		var text, pron string
		var ok bool
		for _, sep := range []string{"=>", "=", ":"} {
			if text, pron, ok = strings.Cut(l, sep); ok {
				break
			}
		}
		text, pron = strings.TrimSpace(text), strings.TrimSpace(pron)
		if !ok || text == "" || pron == "" {
			return "", nil, fmt.Errorf("Line %d should look like “word = pronunciation”.", i+1)
		}
		items = append(items, cartesia.PronunciationItem{Text: text, Pronunciation: pron})
	}
	if len(items) == 0 {
		return "", nil, fmt.Errorf("Add at least one “word = pronunciation” line.")
	}
	return name, items, nil
}

// adminScreen gathers the studio admin's tools in one place.
func (b *Bot) adminScreen(ctx context.Context) screen {
	key := "✅ set"
	if !b.ct.HasAdminKey() {
		key = "❌ missing (set CARTESIA_ADMIN_KEY)"
	}
	var sb strings.Builder
	sb.WriteString(header("🛠", "Admin", "Only studio admins see this"))
	sb.WriteString(field("🔑", "Cartesia admin key", key))
	sb.WriteString(field("👥", "Who can add Ellie to groups", orDefault(b.cfg.GroupMode, GroupsOpen)))
	if total, _, err := b.chats.Get(ctx, allGroups); err == nil {
		c, s := total.usageOn(today())
		sb.WriteString(field("📊", "All groups today", fmtInt(c)+" characters · "+fmtMinutes(s)+" transcribed"))
	} else {
		b.log.Warn("group totals unavailable", "err", err)
	}
	sb.WriteString(field("🎟", "Each user per day", fmtInt(b.lim().DailyChars)+" characters · "+fmtMinutes(b.lim().DailyAudioSecs)+" transcribed"))
	sb.WriteString("\n<b>Commands</b>\n" +
		"/usage · Cartesia credit usage\n" +
		"/groups · today's busiest groups and their settings\n" +
		"/blockgroup <code>id</code> · make Ellie leave a group for good\n" +
		"/unblockgroup <code>id</code> · let a group add Ellie again\n" +
		"/stats · active users, readings, growth\n" +
		"/status · service status")
	k := &keys{}
	if b.cfg.AdminPanelURL != "" {
		k.row(telegram.InlineKeyboardButton{Text: "🖥 Open admin panel", WebApp: &telegram.WebAppInfo{URL: b.cfg.AdminPanelURL}})
	}
	k.row(btn("📊 Stats", "nav:stats"), btn("📈 Credits", "nav:usage")).
		row(btn("👥 Groups", "nav:groups"), btn("🩺 Service status", "nav:status")).
		nav("", "")
	return screen{sb.String(), k.build()}
}

func (b *Bot) statusScreen(ctx context.Context) screen {
	info, err := b.ct.Status(ctx)
	if err != nil {
		return errorScreen(err)
	}
	state := "🟢 All systems normal"
	if !info.OK {
		state = "🔴 Degraded"
	}
	text := header("📊", "Service status", "") + state + "\n" +
		field("🗓", "API version", info.Version) + field("🔗", "Studio uses", cartesia.APIVersion)
	if models, err := b.ct.ListModels(ctx, ""); err == nil {
		var tts, stt []string
		for _, md := range models {
			if md.Capability == "stt" {
				stt = append(stt, md.ID)
			} else {
				tts = append(tts, md.ID)
			}
		}
		text += field(icStudio, "Speech models", strings.Join(tts, ", ")) +
			field(icListen, "Transcription models", strings.Join(stt, ", "))
	}
	return screen{text, (&keys{}).nav("Studio", "nav:studio").build()}
}

func (b *Bot) usageScreen(ctx context.Context, uid int64) screen {
	nav := (&keys{}).nav("Studio", "nav:studio").build()
	if !b.isAdmin(uid) {
		return screen{header("📈", "Usage", "") + icLock + " Usage is visible to studio admins.", nav}
	}
	if !b.ct.HasAdminKey() {
		return screen{header("📈", "Usage", "") + "Usage needs a Cartesia admin key. Set <code>CARTESIA_ADMIN_KEY</code> on the server.", nav}
	}
	end := time.Now().UTC()
	buckets, err := b.ct.CreditUsage(ctx, end.AddDate(0, 0, -28), end, "week")
	if err != nil {
		return errorScreen(err)
	}
	var total int64
	var sb strings.Builder
	sb.WriteString(header("📈", "Usage", "Credits, last 4 weeks"))
	for _, bk := range buckets {
		total += bk.Credits
		fmt.Fprintf(&sb, "%s – %s: <b>%d</b>\n", bk.StartTS.Format("Jan 2"), bk.EndTS.Format("Jan 2"), bk.Credits)
	}
	fmt.Fprintf(&sb, "\nTotal: <b>%d</b> credits", total)
	return screen{sb.String(), nav}
}

// ---------------------------------------------------------------- buttons

func (b *Bot) handleCallback(ctx context.Context, q *telegram.CallbackQuery) {
	if q.Message != nil && isGroupChat(q.Message.Chat.ID) {
		b.handleGroupCallback(ctx, q)
		return
	}
	if !b.allowed(q.From.ID) {
		_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "Ellie Studio is private.")
		return
	}
	if q.Message == nil {
		_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "")
		return
	}
	defer b.join(q.From.ID)
	b.dispatchCallback(ctx, q)
}

// dispatchCallback runs a button. In a group it is reached only for the
// buttons handleGroupCallback allows, and only inside a private view.
func (b *Bot) dispatchCallback(ctx context.Context, q *telegram.CallbackQuery) {
	uid := q.From.ID
	kind, val, _ := strings.Cut(q.Data, ":")
	ack := func(text string) { _ = b.tg.AnswerCallbackQuery(ctx, q.ID, text) }
	// Any button other than Cancel or one that starts a guided step leaves
	// the step, so a later text message is never mistaken for its answer.
	// (Group presses leave private steps alone: those never run in groups.)
	if !isGroupChat(q.Message.Chat.ID) && kind != "cancel" && !(kind == "do" && contains([]string{"search", "clone", "newdict", "rename", "voiceid"}, val)) {
		b.clearFlow(uid)
	}
	// as is a message stand-in so speech handlers can answer in this chat.
	as := &telegram.Message{From: &q.From, Chat: q.Message.Chat}

	switch kind {
	case "news":
		b.setNews(ctx, q, val == "on")
	case "nav":
		ack("")
		b.show(ctx, q, b.navScreen(ctx, uid, val))
	case "help":
		ack("")
		b.show(ctx, q, helpScreen(val))

	// voices
	case "v":
		b.chooseVoice(ctx, q, val)
	case "p":
		ack("Fetching the preview…")
		b.sendPreview(ctx, uid, q.Message.Chat.ID, 0, val)
	case "vc":
		ack("")
		b.show(ctx, q, b.voiceCard(ctx, uid, val))
	case "vn", "vb", "vf", "vm", "vq":
		b.browse(ctx, q, kind, val)
	case "dv":
		ack("")
		b.show(ctx, q, b.deleteConfirmScreen(ctx, uid, val))
	case "dvy":
		b.deleteVoice(ctx, q, val)
	case "sh":
		ack("")
		b.show(ctx, q, b.shareScreen(ctx, uid, val))
	case "rn":
		ack("")
		b.startRename(ctx, q.Message.Chat.ID, uid, val)
	case "loc":
		ack("Creating your localized voice…")
		if q.Message.Text == "" { // pressed under audio: draw into a fresh message
			if t := b.sendScreen(ctx, q.Message.Chat.ID, 0, screen{icWait + " Starting…", nil}); t != nil {
				b.runLocalize(ctx, uid, t, val)
			}
			return
		}
		b.runLocalize(ctx, uid, q.Message, val)

	// style
	case "e":
		if err := b.setEmotion(uid, val); err != nil {
			ack(err.Error())
			return
		}
		ack(icOK + " Emotion: " + orDefault(map[string]string{"auto": "natural"}[val], val))
		b.show(ctx, q, b.emotionScreen(uid, !contains(featuredEmotions, val) && val != "auto"))
	case "s", "vo":
		vol := kind == "vo"
		if err := b.setFactor(uid, vol, val); err != nil {
			ack(err.Error())
			return
		}
		ack(icOK + " " + map[bool]string{false: "Speed ", true: "Volume "}[vol] + fmtFactor(mustFloat(val)))
		b.show(ctx, q, b.rangeScreen(uid, vol))
	case "m":
		if err := b.setModel(ctx, uid, val); err != nil {
			ack(err.Error())
			return
		}
		ack(icOK + " Model: " + val)
		b.show(ctx, q, b.modelScreen(ctx, uid))
	case "l":
		if err := b.setLang(uid, val); err != nil {
			ack(err.Error())
			return
		}
		ack(icOK + " Language: " + langLabel(b.store.Get(uid).Language))
		b.show(ctx, q, b.langScreen(ctx, uid))
	case "f":
		b.setOutput(uid, val == "file")
		ack(icOK + " Output: " + outputLabel(val == "file"))
		b.show(ctx, q, b.outputScreen(uid))
	case "d":
		id, name := "", ""
		if val != "-" {
			d, err := b.ct.GetPronunciationDict(ctx, val)
			if err == nil {
				if ok, cerr := b.canUseDict(ctx, uid, *d); cerr != nil {
					err = cerr
				} else if !ok {
					err = errPrivateDict
				}
			}
			if err != nil {
				ack("")
				b.show(ctx, q, errorScreen(err))
				return
			}
			id, name = d.ID, d.Name
		}
		if _, err := b.store.Update(uid, func(s *Settings) { s.DictID, s.DictName = id, name }); err != nil {
			ack("")
			b.show(ctx, q, errorScreen(err))
			return
		}
		ack(icOK + " Pronunciation: " + orDefault(name, "none"))
		b.show(ctx, q, b.dictsScreen(ctx, uid))

	// actions
	case "do":
		b.action(ctx, q, as, val)
	case "reset":
		if err := b.store.Reset(uid); err != nil {
			ack("")
			b.show(ctx, q, errorScreen(err))
			return
		}
		ack(icOK + " Settings reset")
		b.show(ctx, q, b.homeScreen(uid))
	case "cancel":
		b.clearFlow(uid)
		ack("Cancelled")
		b.show(ctx, q, cancelledScreen())
	default:
		ack("")
	}
}

// withWarning keeps the user on the picker and explains what went wrong.
func withWarning(s screen, err error) screen {
	s.text += "\n\n" + icWarn + " " + explain(err)
	return s
}

func mustFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// navScreen maps "nav:<name>" to its screen.
func (b *Bot) navScreen(ctx context.Context, uid int64, name string) screen {
	switch name {
	case "style":
		return b.styleScreen(uid)
	case "emotion":
		return b.emotionScreen(uid, false)
	case "emotions":
		return b.emotionScreen(uid, true)
	case "speed":
		return b.rangeScreen(uid, false)
	case "volume":
		return b.rangeScreen(uid, true)
	case "lang":
		return b.langScreen(ctx, uid)
	case "model":
		return b.modelScreen(ctx, uid)
	case "output":
		return b.outputScreen(uid)
	case "voices":
		return b.voicesScreen(ctx, uid, b.resetSession(uid, "", nil))
	case "studio":
		return b.studioScreen(uid)
	case "dicts":
		return b.dictsScreen(ctx, uid)
	case "create":
		return b.createScreen(uid)
	case "localize":
		return b.localizeScreen(ctx, uid)
	case "help":
		return helpScreen("")
	case "status":
		return b.statusScreen(ctx)
	case "usage":
		return b.usageScreen(ctx, uid)
	case "profile":
		return b.profileScreen(uid)
	case "invite":
		return b.inviteScreen(uid)
	case "admin", "groups", "stats":
		if !b.isOwner(uid) {
			return b.homeScreen(uid)
		}
		switch name {
		case "groups":
			return b.groupsScreen(ctx)
		case "stats":
			return b.statsScreen(ctx)
		}
		return b.adminScreen(ctx)
	case "reset":
		return resetScreen()
	}
	return b.homeScreen(uid)
}

// action handles "do:<what>" buttons.
func (b *Bot) action(ctx context.Context, q *telegram.CallbackQuery, as *telegram.Message, what string) {
	uid, chat := q.From.ID, q.Message.Chat.ID
	ack := func(text string) { _ = b.tg.AnswerCallbackQuery(ctx, q.ID, text) }
	switch what {
	case "sample":
		ack("")
		s := b.store.Get(uid)
		b.speak(ctx, as, fmt.Sprintf("Hi, I'm %s. This is how I sound in Ellie Studio. Send me anything and I'll read it out loud.",
			orDefault(s.VoiceName, "your new voice")))
	case "again":
		text := b.lastSpoken(uid)
		if text == "" {
			ack("Send me some text first")
			return
		}
		ack(icAgain + " One more time")
		b.speak(ctx, as, text)
	case "readback":
		text := b.lastTranscript(uid)
		if text == "" {
			ack("Nothing to read yet")
			return
		}
		ack("")
		b.speak(ctx, as, text)
	case "transcribe":
		ack("")
		b.show(ctx, q, transcribePrompt())
	case "search":
		ack("")
		b.startFlow(ctx, chat, uid, flowSearch)
	case "clone":
		ack("")
		b.startFlow(ctx, chat, uid, flowCloneClip)
	case "newdict":
		ack("")
		b.startFlow(ctx, chat, uid, flowDictName)
	case "rename":
		ack("")
		b.startFlow(ctx, chat, uid, flowRename)
	case "voiceid":
		ack("")
		b.startFlow(ctx, chat, uid, flowVoiceID)
	case "mine":
		ack("")
		owned := true
		b.openVoices(ctx, chat, uid, "", &owned)
	default:
		ack("")
	}
}
