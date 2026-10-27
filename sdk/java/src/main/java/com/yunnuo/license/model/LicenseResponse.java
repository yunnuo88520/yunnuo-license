package com.yunnuo.license.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import java.util.Map;

import java.time.Instant;

@JsonIgnoreProperties(ignoreUnknown=true)
public record LicenseResponse(
		@JsonProperty("license_mode") String licenseMode,
		@JsonProperty("can_use") boolean canUse,
		@JsonProperty("can_update") boolean canUpdate,
		@JsonProperty("usage_expired_at") Instant usageExpiredAt,
		@JsonProperty("updates_expired_at") Instant updatesExpiredAt,
        @JsonProperty("license_no") String licenseNo,
        @JsonProperty("status") String status,
        @JsonProperty("expired_at") Instant expiredAt,
        @JsonProperty("grace_until") Instant graceUntil,
        @JsonProperty("license_token") String licenseToken,
        @JsonProperty("offline_token") String offlineToken,
        @JsonProperty("server_time") Instant serverTime,
 @JsonProperty("authorization_terms") Map<String,Object> authorizationTerms,
 @JsonProperty("bind_mode") String bindMode,
 @JsonProperty("bind_value") String bindValue) {
}
