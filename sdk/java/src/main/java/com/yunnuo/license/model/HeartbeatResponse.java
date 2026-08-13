package com.yunnuo.license.model;

import com.fasterxml.jackson.annotation.JsonProperty;

import java.time.Instant;

public record HeartbeatResponse(
        @JsonProperty("accepted") boolean accepted,
        @JsonProperty("server_time") Instant serverTime) {
}
