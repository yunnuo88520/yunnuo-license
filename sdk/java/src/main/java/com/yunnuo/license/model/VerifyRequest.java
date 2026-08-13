package com.yunnuo.license.model;

import com.fasterxml.jackson.annotation.JsonProperty;

public record VerifyRequest(
        @JsonProperty("license_no") String licenseNo,
        @JsonProperty("bind_mode") String bindMode,
        @JsonProperty("bind_value") String bindValue,
        @JsonProperty("license_token") String licenseToken) {
}
