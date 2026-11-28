package bot

// aliases.go re-exports what moved to internal/store, so the rest of the bot
// (and its callers) keep their names while persistence lives in one package.

import "ellestudio/internal/store"

type DB = store.DB

var OpenDatabase = store.OpenDatabase

const pgTimeout = store.PgTimeout
