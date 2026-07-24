// Fixture: plugin that throws during module evaluation.
// Used to test Restart()'s re-eval error path — when a plugin path
// causes a bridge eval error, Restart must stop the bridge, nil it out,
// and return the error.
export const runtime = "node";

throw new Error("intentional eval error for restart test");
