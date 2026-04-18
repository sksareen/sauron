export async function register() {
  if (process.env.NEXT_RUNTIME === "nodejs") {
    const { startVaultWatcher } = await import("./lib/vault-watcher");
    startVaultWatcher();
  }
}
