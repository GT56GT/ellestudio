package bot

// aliases.go re-exports what moved to internal/store, so the rest of the bot
// (and its callers) keep their names while persistence lives in one package.

import "ellestudio/internal/store"

type DB = store.DB

var OpenDatabase = store.OpenDatabase

const pgTimeout = store.PgTimeout

type Settings = store.Settings
type Ban = store.Ban
type Usage = store.Usage
type Store = store.SettingsStore

var OpenStore = store.OpenStore

func overLimit(used, limit int) int { return store.OverLimit(used, limit) }

// writeFileAtomic is shared by the file-backed registries still in this package.
var writeFileAtomic = store.WriteFileAtomic
