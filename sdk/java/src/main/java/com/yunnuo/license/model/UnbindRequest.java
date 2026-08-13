package com.yunnuo.license.model;

import com.fasterxml.jackson.annotation.JsonProperty;

public record UnbindRequest(
        @JsonProperty("license_no") String licenseNo,
        @JsonProperty("bind_mode") String bindMode,
        @JsonProperty("bind_value") String bindValue,
        @JsonProperty("reason") String reason) {
}
