package com.yunnuo.license.model;

import com.fasterxml.jackson.annotation.JsonProperty;

public record ActivateRequest(
        @JsonProperty("card_code") String cardCode,
        @JsonProperty("bind_mode") String bindMode,
        @JsonProperty("bind_value") String bindValue,
        @JsonProperty("device_name") String deviceName,
        @JsonProperty("client_version") String clientVersion) {
}
