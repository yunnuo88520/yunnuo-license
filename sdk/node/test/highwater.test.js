import assert from "node:assert/strict";
import test from "node:test";

import { HighWaterGuard, VerificationCode, isVerificationErrorCode } from "../src/index.js";

test("high-water guard tolerates small adjustment and rejects rollback", async () => {
  let saved = null;
  const store = {
    async load() {
      return saved;
    },
    async save(value) {
      saved = value;
    },
  };
  const guard = new HighWaterGuard({ store, allowedRollbackMs: 60_000 });
  const base = new Date("2026-08-12T03:00:00Z");
  await guard.checkAndUpdate(base);
  await guard.checkAndUpdate(new Date(base.getTime() - 30_000));
  await assert.rejects(
    guard.checkAndUpdate(new Date(base.getTime() - 120_000)),
    (error) => isVerificationErrorCode(error, VerificationCode.CLOCK_ROLLBACK),
  );
  const future = new Date(base.getTime() + 3_600_000);
  await guard.checkAndUpdate(future);
  assert.equal(saved.toISOString(), future.toISOString());
});

test("high-water guard serializes concurrent updates", async () => {
  let saved = null;
  const store = {
    async load() {
      await new Promise((resolve) => setTimeout(resolve, 2));
      return saved;
    },
    async save(value) {
      saved = value;
    },
  };
  const guard = new HighWaterGuard({ store });
  const first = new Date("2026-08-12T03:00:00Z");
  const second = new Date("2026-08-12T04:00:00Z");
  await Promise.all([guard.checkAndUpdate(first), guard.checkAndUpdate(second)]);
  assert.equal(saved.toISOString(), second.toISOString());
});
