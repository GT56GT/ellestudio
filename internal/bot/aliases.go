package bot

// aliases.go re-exports what moved to internal/store, so the rest of the bot
// (and its callers) keep their names while persistence lives in one package.

import "ellestudio/internal/store"

var OpenDatabase = store.OpenDatabase

const pgTimeout = store.PgTimeout

type Settings = store.Settings
type Ban = store.Ban
type Usage = store.Usage
type Store = store.SettingsStore

var OpenStore = store.OpenStore

// Ownership registry.
type (
	Owners = store.Owners
	Cursor = store.Cursor
)

const (
	kindVoice = store.KindVoice
	kindDict  = store.KindDict
)

var (
	OpenFileOwners = store.OpenFileOwners
	cursorOf       = store.CursorOf
	parseCursor    = store.ParseCursor
)

// Group registry.
type (
	Chats         = store.Chats
	ChatInfo      = store.ChatInfo
	GroupSettings = store.GroupSettings
	GroupLimits   = store.GroupLimits
)

const (
	allGroups  = store.AllGroups
	chatMember = store.ChatMember
	chatLeft   = store.ChatLeft
)

var (
	errGroupLimit  = store.ErrGroupLimit
	errGroupsLimit = store.ErrGroupsLimit
	OpenFileChats  = store.OpenFileChats
)

// Usage events.
type (
	Metrics  = store.Metrics
	Event    = store.Event
	DayStats = store.DayStats
)

const (
	evStart       = store.EvStart
	evReading     = store.EvReading
	evTranscribe  = store.EvTranscribe
	evLimit       = store.EvLimit
	evGuestInvite = store.EvGuestInvite

	srcDirect   = store.SrcDirect
	srcGroup    = store.SrcGroup
	srcGuest    = store.SrcGuest
	srcReferral = store.SrcReferral
	srcShared   = store.SrcShared
)

var newMemMetrics = store.NewMemMetrics

// Studio-wide budget.
type Budget = store.Budget

var (
	errBudgetUnknown = store.ErrBudgetUnknown
	newMemBudget     = store.NewMemBudget
)

// Panel-facing data.
type (
	Status         = store.Status
	StatusLimits   = store.StatusLimits
	Limits         = store.Limits
	LimitOverrides = store.LimitOverrides
	AdminJob       = store.AdminJob
	AdminQueue     = store.AdminQueue
	Broadcast      = store.Broadcast
	BroadcastQueue = store.BroadcastQueue
)

const (
	JobDeleteVoice = store.JobDeleteVoice
	JobDeleteDict  = store.JobDeleteDict
)

var BroadcastHTML = store.BroadcastHTML
