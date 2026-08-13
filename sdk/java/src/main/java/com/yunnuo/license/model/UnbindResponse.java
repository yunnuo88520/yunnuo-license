package com.yunnuo.license.model;

import com.fasterxml.jackson.annotation.JsonProperty;

import java.time.Instant;

public record UnbindResponse(
        @JsonProperty("unbound") boolean unbound,
        @JsonProperty("license_no") String licenseNo,
        @JsonProperty("server_time") Instant serverTime) {
}
