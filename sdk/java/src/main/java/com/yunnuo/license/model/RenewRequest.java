package com.yunnuo.license.model;

import com.fasterxml.jackson.annotation.JsonProperty;

public record RenewRequest(
        @JsonProperty("license_no") String licenseNo,
        @JsonProperty("renew_card_code") String renewCardCode,
        @JsonProperty("bind_mode") String bindMode,
        @JsonProperty("bind_value") String bindValue) {
}
