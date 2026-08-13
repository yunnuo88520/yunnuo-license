package com.yunnuo.license.model;

import com.fasterxml.jackson.annotation.JsonProperty;

import java.time.Instant;

public record OfflineLicenseClaims(
        @JsonProperty("version") int version,
        @JsonProperty("key_version") int keyVersion,
        @JsonProperty("license_no") String licenseNo,
        @JsonProperty("product_id") String productId,
        @JsonProperty("product_code") String productCode,
        @JsonProperty("product_name") String productName,
        @JsonProperty("app_key") String appKey,
        @JsonProperty("bind_mode") String bindMode,
        @JsonProperty("machine_code") String machineCode,
        @JsonProperty("issued_at") Instant issuedAt,
        @JsonProperty("expired_at") Instant expiredAt,
        @JsonProperty("is_permanent") boolean permanent) {
}
