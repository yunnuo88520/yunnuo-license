package com.yunnuo.license.model;

import com.fasterxml.jackson.annotation.JsonProperty;

import java.time.Instant;

public record LicenseResponse(
        @JsonProperty("license_no") String licenseNo,
        @JsonProperty("status") String status,
        @JsonProperty("expired_at") Instant expiredAt,
        @JsonProperty("grace_until") Instant graceUntil,
        @JsonProperty("license_token") String licenseToken,
        @JsonProperty("offline_token") String offlineToken,
        @JsonProperty("server_time") Instant serverTime) {
}
