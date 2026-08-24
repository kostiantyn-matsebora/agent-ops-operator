
// DELIBERATE FAILURE, for sdlc-setup task 2.8. Removed in the next commit on
// this branch, which is what proves the job goes green again for the right
// reason rather than by the break being forgotten.
func TestDeliberateBreakForCI(t *testing.T) {
	t.Fatal("deliberate failure: proving CI attributes a module failure to signals/cron")
}
