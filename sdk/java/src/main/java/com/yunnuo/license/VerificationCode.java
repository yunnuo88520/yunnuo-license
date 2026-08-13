package com.yunnuo.license;

public final class VerificationCode {
    public static final String INVALID_FILE = "INVALID_FILE";
    public static final String INVALID_SIGNATURE = "INVALID_SIGNATURE";
    public static final String WRONG_PRODUCT = "WRONG_PRODUCT";
    public static final String WRONG_LICENSE = "WRONG_LICENSE";
    public static final String BINDING_MISMATCH = "BINDING_MISMATCH";
    public static final String LICENSE_EXPIRED = "LICENSE_EXPIRED";
    public static final String OFFLINE_WINDOW_EXPIRED = "OFFLINE_WINDOW_EXPIRED";
    public static final String ISSUED_IN_FUTURE = "ISSUED_IN_FUTURE";
    public static final String CLOCK_ROLLBACK = "CLOCK_ROLLBACK";

    private VerificationCode() {
    }
}
