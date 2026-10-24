package com.yunnuo.license.model;

import com.fasterxml.jackson.annotation.JsonProperty;

import java.time.Instant;

public record OfflineCacheClaims(
        @JsonProperty("license_mode") String licenseMode,
        @JsonProperty("usage_expired_at") Instant usageExpiredAt,
        @JsonProperty("updates_expired_at") Instant updatesExpiredAt,
        @JsonProperty("type") String type,
        @JsonProperty("license_no") String licenseNo,
        @JsonProperty("product_code") String productCode,
        @JsonProperty("bind_mode") String bindMode,
        @JsonProperty("bind_hash") String bindHash,
        @JsonProperty("bind_digest") String bindDigest,
        @JsonProperty("license_expired_at") Instant licenseExpiredAt,
        @JsonProperty("offline_until") Instant offlineUntil,
        @JsonProperty("issued_at") Instant issuedAt,
        @JsonProperty("key_version") int keyVersion) {
}
