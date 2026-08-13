package com.yunnuo.license;

public class LicenseSDKException extends RuntimeException {
    public LicenseSDKException(String message) {
        super(message);
    }

    public LicenseSDKException(String message, Throwable cause) {
        super(message + ": " + cause.getMessage(), cause);
    }
}
