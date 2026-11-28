package store

import (
	"context"
	"testing"
)

func TestOverridesRoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	cleanup := func() { _, _ = db.Pool().Exec(ctx, `DELETE FROM ellie_config WHERE key = 'limits'`) }
	cleanup()
	t.Cleanup(cleanup)

	if o, err := db.LoadOverrides(ctx); err != nil || o.DailyChars != nil || o.Paused {
		t.Fatalf("empty overrides = %+v %v", o, err)
	}
	n := 1234
	if before, err := SaveOverrides(ctx, db.Pool(), LimitOverrides{DailyChars: &n, Paused: true}, 1); err != nil || before.DailyChars != nil {
		t.Fatalf("first save: %+v %v", before, err)
	}
	o, err := db.LoadOverrides(ctx)
	if err != nil || o.DailyChars == nil || *o.DailyChars != 1234 || !o.Paused {
		t.Fatalf("overrides = %+v %v", o, err)
	}
	m := 5
	before, err := SaveOverrides(ctx, db.Pool(), LimitOverrides{DailyDicts: &m}, 2)
	if err != nil || before.DailyChars == nil || *before.DailyChars != 1234 {
		t.Fatalf("second save returned %+v %v", before, err)
	}
}

func TestOverridesApplyAndValidate(t *testing.T) {
	n, cool := 7, 90
	base := Limits{DailyChars: 100, GroupLimits: GroupLimits{Chars: 10}}
	l := LimitOverrides{DailyChars: &n, GroupChars: &n, GroupCooldownSecs: &cool, Paused: true}.Apply(base)
	if l.DailyChars != 7 || l.GroupLimits.Chars != 7 || l.GroupCooldown.Seconds() != 90 || !l.Paused {
		t.Fatalf("apply = %+v", l)
	}
	zero := 0
	if err := (LimitOverrides{MaxTTSChars: &zero}).Validate(); err == nil {
		t.Fatal("longest text of 0 must be rejected")
	}
	neg := -1
	if err := (LimitOverrides{DailyChars: &neg}).Validate(); err == nil {
		t.Fatal("negative override must be rejected")
	}
}
