package com.yunnuo.license;

import java.time.Instant;
import java.util.Optional;

public interface HighWaterStore {
    Optional<Instant> load();

    void save(Instant value);
}
