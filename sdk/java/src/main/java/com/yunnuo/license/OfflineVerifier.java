package com.yunnuo.license;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.yunnuo.license.model.OfflineCacheClaims;
import com.yunnuo.license.model.OfflineExpectation;
import com.yunnuo.license.model.OfflineLicenseClaims;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.security.GeneralSecurityException;
import java.security.KeyFactory;
import java.security.MessageDigest;
import java.security.PublicKey;
import java.security.Signature;
import java.security.interfaces.RSAPublicKey;
import java.security.spec.X509EncodedKeySpec;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.util.Base64;
import java.util.Locale;
import java.util.Objects;

public final class OfflineVerifier {
    private final RSAPublicKey publicKey;
    private final String productCode;
    private final ObjectMapper objectMapper;
    private final Clock clock;
    private final Duration clockSkew;

    public OfflineVerifier(String publicKeyPem, String productCode) {
        this(builder(publicKeyPem, productCode));
    }

    private OfflineVerifier(Builder builder) {
        this.publicKey = parsePublicKey(builder.publicKeyPem);
        this.productCode = requireText(builder.productCode, "product code");
        this.objectMapper = Objects.requireNonNull(builder.objectMapper, "objectMapper");
        this.clock = Objects.requireNonNull(builder.clock, "clock");
        this.clockSkew = Objects.requireNonNull(builder.clockSkew, "clockSkew");
        if (clockSkew.isNegative()) {
            throw new IllegalArgumentException("ynlicense: clock skew cannot be negative");
        }
    }

    public static Builder builder(String publicKeyPem, String productCode) {
        return new Builder(publicKeyPem, productCode);
    }

    public OfflineCacheClaims verifyOfflineToken(String token, OfflineExpectation expected) {
        Objects.requireNonNull(expected, "expected");
        OfflineCacheClaims claims = verifySignedJson(token, OfflineCacheClaims.class);
        if (!"offline_cache".equals(claims.type()) || isBlank(claims.licenseNo()) || claims.offlineUntil() == null || claims.issuedAt() == null) {
            fail(VerificationCode.INVALID_FILE, "invalid offline cache claims");
        }
        if (!equalFoldTrim(claims.productCode(), productCode)) {
            fail(VerificationCode.WRONG_PRODUCT, "product code does not match");
        }
        if (!isBlank(expected.licenseNo()) && !claims.licenseNo().trim().equals(expected.licenseNo().trim())) {
            fail(VerificationCode.WRONG_LICENSE, "license number does not match");
        }
        if (!equalFoldTrim(claims.bindMode(), expected.bindMode()) || isBlank(claims.bindDigest())) {
            fail(VerificationCode.BINDING_MISMATCH, "binding mode or digest does not match");
        }
        byte[] actual = claims.bindDigest().toLowerCase(Locale.ROOT).getBytes(StandardCharsets.US_ASCII);
        byte[] wanted = bindingDigest(expected.bindMode(), expected.bindValue()).getBytes(StandardCharsets.US_ASCII);
        if (!MessageDigest.isEqual(actual, wanted)) {
            fail(VerificationCode.BINDING_MISMATCH, "binding value does not match");
        }
        Instant now = clock.instant();
        if (claims.issuedAt().isAfter(now.plus(clockSkew))) {
            fail(VerificationCode.ISSUED_IN_FUTURE, "token issue time is in the future");
        }
        if (claims.licenseExpiredAt() != null && !now.isBefore(claims.licenseExpiredAt())) {
            fail(VerificationCode.LICENSE_EXPIRED, "license has expired");
        }
        if (!now.isBefore(claims.offlineUntil())) {
            fail(VerificationCode.OFFLINE_WINDOW_EXPIRED, "offline window has expired");
        }
        return claims;
    }

    public OfflineLicenseClaims verifyLicenseFile(byte[] content, String machineCode) {
        JsonNode file;
        try {
            file = objectMapper.readTree(content);
        } catch (IOException error) {
            throw new VerificationException(VerificationCode.INVALID_FILE, "file is not valid JSON");
        }
        if (file == null || !"yn-license-key".equals(file.path("format").asText()) || file.path("version").asInt() != 1 || file.path("token").asText().isBlank()) {
            fail(VerificationCode.INVALID_FILE, "unsupported file format or version");
        }
        OfflineLicenseClaims claims = verifySignedJson(file.path("token").asText(), OfflineLicenseClaims.class);
        if (claims.version() != 1 || isBlank(claims.licenseNo()) || !"device".equals(claims.bindMode()) || claims.issuedAt() == null) {
            fail(VerificationCode.INVALID_FILE, "invalid offline license claims");
        }
        if (!equalFoldTrim(claims.productCode(), productCode)) {
            fail(VerificationCode.WRONG_PRODUCT, "product code does not match");
        }
        if (!equalFoldTrim(claims.machineCode(), machineCode)) {
            fail(VerificationCode.BINDING_MISMATCH, "machine code does not match");
        }
        Instant now = clock.instant();
        if (claims.issuedAt().isAfter(now.plus(clockSkew))) {
            fail(VerificationCode.ISSUED_IN_FUTURE, "license issue time is in the future");
        }
        if (!claims.permanent() && (claims.expiredAt() == null || !now.isBefore(claims.expiredAt()))) {
            fail(VerificationCode.LICENSE_EXPIRED, "license has expired");
        }
        return claims;
    }

    public OfflineLicenseClaims verifyLicenseFile(String content, String machineCode) {
        return verifyLicenseFile(content.getBytes(StandardCharsets.UTF_8), machineCode);
    }

    public static String bindingDigest(String bindMode, String bindValue) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            digest.update(normalize(bindMode).getBytes(StandardCharsets.UTF_8));
            digest.update((byte) 0);
            digest.update(normalize(bindValue).getBytes(StandardCharsets.UTF_8));
            return java.util.HexFormat.of().formatHex(digest.digest());
        } catch (GeneralSecurityException impossible) {
            throw new IllegalStateException(impossible);
        }
    }

    private <T> T verifySignedJson(String token, Class<T> type) {
        String[] parts = token == null ? new String[0] : token.split("\\.", -1);
        if (parts.length != 2) {
            fail(VerificationCode.INVALID_SIGNATURE, "invalid signed token");
        }
        try {
            byte[] body = Base64.getUrlDecoder().decode(parts[0]);
            byte[] signatureBytes = Base64.getUrlDecoder().decode(parts[1]);
            Signature signature = Signature.getInstance("SHA256withRSA");
            signature.initVerify(publicKey);
            signature.update(body);
            if (!signature.verify(signatureBytes)) {
                fail(VerificationCode.INVALID_SIGNATURE, "signature verification failed");
            }
            return objectMapper.readValue(body, type);
        } catch (VerificationException error) {
            throw error;
        } catch (IllegalArgumentException | GeneralSecurityException error) {
            throw new VerificationException(VerificationCode.INVALID_SIGNATURE, "signature verification failed");
        } catch (IOException error) {
            throw new VerificationException(VerificationCode.INVALID_FILE, "signed claims are not valid JSON");
        }
    }

    private static RSAPublicKey parsePublicKey(String pem) {
        try {
            String normalized = requireText(pem, "public key")
                    .replace("-----BEGIN PUBLIC KEY-----", "")
                    .replace("-----END PUBLIC KEY-----", "")
                    .replaceAll("\\s", "");
            byte[] encoded = Base64.getDecoder().decode(normalized);
            PublicKey key = KeyFactory.getInstance("RSA").generatePublic(new X509EncodedKeySpec(encoded));
            if (!(key instanceof RSAPublicKey rsaKey)) {
                throw new IllegalArgumentException();
            }
            return rsaKey;
        } catch (IllegalArgumentException | GeneralSecurityException error) {
            throw new IllegalArgumentException("ynlicense: invalid RSA public key", error);
        }
    }

    private static boolean equalFoldTrim(String left, String right) {
        return normalize(left).equals(normalize(right));
    }

    private static String normalize(String value) {
        return value == null ? "" : value.trim().toLowerCase(Locale.ROOT);
    }

    private static boolean isBlank(String value) {
        return value == null || value.isBlank();
    }

    private static String requireText(String value, String name) {
        if (isBlank(value)) {
            throw new IllegalArgumentException("ynlicense: " + name + " is required");
        }
        return value.trim();
    }

    private static void fail(String code, String message) {
        throw new VerificationException(code, message);
    }

    public static final class Builder {
        private final String publicKeyPem;
        private final String productCode;
        private ObjectMapper objectMapper = LicenseClient.defaultObjectMapper();
        private Clock clock = Clock.systemUTC();
        private Duration clockSkew = Duration.ofMinutes(5);

        private Builder(String publicKeyPem, String productCode) {
            this.publicKeyPem = publicKeyPem;
            this.productCode = productCode;
        }

        public Builder objectMapper(ObjectMapper value) {
            this.objectMapper = value;
            return this;
        }

        public Builder clock(Clock value) {
            this.clock = value;
            return this;
        }

        public Builder clockSkew(Duration value) {
            this.clockSkew = value;
            return this;
        }

        public OfflineVerifier build() {
            return new OfflineVerifier(this);
        }
    }
}
