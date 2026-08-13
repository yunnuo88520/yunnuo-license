import { VerificationError } from "./errors.js";
import { VerificationCode } from "./offline.js";

export class HighWaterGuard {
  #store;
  #allowedRollbackMs;
  #queue = Promise.resolve();

  constructor({ store, allowedRollbackMs = 0 }) {
    if (!store || typeof store.load !== "function" || typeof store.save !== "function") {
      throw new TypeError("ynlicense: high-water store with load and save is required");
    }
    if (!Number.isFinite(allowedRollbackMs) || allowedRollbackMs < 0) {
      throw new TypeError("ynlicense: allowedRollbackMs cannot be negative");
    }
    this.#store = store;
    this.#allowedRollbackMs = allowedRollbackMs;
  }

  checkAndUpdate(current = new Date()) {
    const operation = this.#queue.then(() => this.#checkAndUpdate(current));
    this.#queue = operation.catch(() => undefined);
    return operation;
  }

  async #checkAndUpdate(current) {
    const now = parseDate(current, "current time");
    const loaded = await this.#store.load();
    const last = loaded == null || loaded === "" ? null : parseDate(loaded, "saved high-water time");
    if (last) {
      if (now.getTime() + this.#allowedRollbackMs < last.getTime()) {
        throw new VerificationError(VerificationCode.CLOCK_ROLLBACK, "current time is earlier than the saved high-water time");
      }
      if (now.getTime() <= last.getTime()) return;
    }
    await this.#store.save(now);
  }
}

function parseDate(value, name) {
  const date = value instanceof Date ? new Date(value.getTime()) : new Date(value);
  if (Number.isNaN(date.getTime())) throw new TypeError(`ynlicense: ${name} is invalid`);
  return date;
}
