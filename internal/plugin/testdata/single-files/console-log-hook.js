// Fixture: hook that calls console.log() inside its callback.
// console.log is already patched to stderr by bridge.js. This fixture
// is a regression guard (#968) ensuring the new process.stdout.write
// patch does not break the existing console.log redirection.
export const runtime = "node";

export default function(alloy) {
    alloy.hook("onBuildComplete", { data: true }, (payload) => {
        console.log("debug message from hook");
        return { ...payload, hookProcessed: true };
    });
}
