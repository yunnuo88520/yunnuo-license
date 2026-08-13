package com.yunnuo.license;

import java.time.Duration;
import java.time.Instant;
import java.util.Objects;
import java.util.Optional;

public final class HighWaterGuard {
    private final HighWaterStore store;
    private final Duration allowedRollback;

    public HighWaterGuard(HighWaterStore store, Duration allowedRollback) {
        this.store = Objects.requireNonNull(store, "store");
        this.allowedRollback = Objects.requireNonNull(allowedRollback, "allowedRollback");
        if (allowedRollback.isNegative()) {
            throw new IllegalArgumentException("ynlicense: allowed rollback cannot be negative");
        }
    }

    public synchronized void checkAndUpdate(Instant current) {
        Objects.requireNonNull(current, "current");
        Optional<Instant> loaded = Objects.requireNonNull(store.load(), "store.load()");
        if (loaded.isPresent()) {
            Instant last = loaded.get();
            if (current.plus(allowedRollback).isBefore(last)) {
                throw new VerificationException(VerificationCode.CLOCK_ROLLBACK,
                        "current time is earlier than the saved high-water time");
            }
            if (!current.isAfter(last)) {
                return;
            }
        }
        store.save(current);
    }
}
