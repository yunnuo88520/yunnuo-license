package com.yunnuo.license;

public final class VerificationException extends RuntimeException {
    private final String code;

    public VerificationException(String code, String message) {
        super("ynlicense: offline verification " + code + ": " + message);
        this.code = code;
    }

    public String code() {
        return code;
    }

    public boolean hasCode(String expected) {
        return code.equals(expected);
    }
}
