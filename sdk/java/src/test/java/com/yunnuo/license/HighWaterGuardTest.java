package com.yunnuo.license;

import org.junit.jupiter.api.Test;

import java.time.Duration;
import java.time.Instant;
import java.util.Optional;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class HighWaterGuardTest {
    @Test
    void detectsClockRollbackAndAdvancesHighWater() {
        AtomicReference<Instant> value = new AtomicReference<>();
        HighWaterStore store = new HighWaterStore() {
            @Override
            public Optional<Instant> load() {
                return Optional.ofNullable(value.get());
            }

            @Override
            public void save(Instant next) {
                value.set(next);
            }
        };
        HighWaterGuard guard = new HighWaterGuard(store, Duration.ofMinutes(1));
        Instant base = Instant.parse("2026-08-12T03:00:00Z");
        guard.checkAndUpdate(base);
        guard.checkAndUpdate(base.minusSeconds(30));
        VerificationException rollback = assertThrows(VerificationException.class,
                () -> guard.checkAndUpdate(base.minusSeconds(120)));
        assertTrue(rollback.hasCode(VerificationCode.CLOCK_ROLLBACK));
        guard.checkAndUpdate(base.plusSeconds(3600));
        assertEquals(base.plusSeconds(3600), value.get());
    }
}
