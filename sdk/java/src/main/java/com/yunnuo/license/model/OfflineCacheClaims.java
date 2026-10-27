package com.yunnuo.license.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import java.util.Map;

import java.time.Instant;

@JsonIgnoreProperties(ignoreUnknown=true)
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
        @JsonProperty("key_version") int keyVersion,
 @JsonProperty("instance_id") String instanceId,
 @JsonProperty("seat_id") String seatId,
 @JsonProperty("session_id") String sessionId,
 @JsonProperty("purpose") String purpose,
 @JsonProperty("can_use") boolean canUse,
 @JsonProperty("authorization_terms") Map<String,Object> authorizationTerms) {
}
