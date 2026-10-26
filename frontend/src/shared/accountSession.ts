// User center and agent workspace are two views of the same signed-in account.
const names = ["yn.account_token", "yn.user_token", "yn.agent_token"];
export function accountToken(legacy: "user" | "agent") {
  return (
    sessionStorage.getItem(names[0]) ||
    sessionStorage.getItem(`yn.${legacy}_token`) ||
    ""
  );
}
export function saveAccountToken(value: string) {
  for (const name of names) sessionStorage.setItem(name, value);
}
export function clearAccountToken() {
  for (const name of names) sessionStorage.removeItem(name);
}
