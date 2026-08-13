package com.yunnuo.license;

public final class APIException extends RuntimeException {
    private final String code;
    private final int httpStatus;
    private final String requestId;

    public APIException(String code, String message, int httpStatus, String requestId) {
        super("ynlicense: " + code + ": " + message +
                (requestId == null || requestId.isBlank() ? "" : " (request_id=" + requestId + ")"));
        this.code = code;
        this.httpStatus = httpStatus;
        this.requestId = requestId == null ? "" : requestId;
    }

    public String code() {
        return code;
    }

    public int httpStatus() {
        return httpStatus;
    }

    public String requestId() {
        return requestId;
    }

    public boolean hasCode(String expected) {
        return code.equals(expected);
    }
}
