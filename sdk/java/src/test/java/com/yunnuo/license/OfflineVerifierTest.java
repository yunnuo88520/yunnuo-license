package com.yunnuo.license;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.yunnuo.license.model.OfflineCacheClaims;
import com.yunnuo.license.model.OfflineExpectation;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.PrivateKey;
import java.security.Signature;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class OfflineVerifierTest {
    private static final Instant NOW = Instant.parse("2026-08-12T03:00:00Z");
    private static final ObjectMapper MAPPER = LicenseClient.defaultObjectMapper();
    private static KeyPair keyPair;
    private static String publicKeyPem;

    @BeforeAll
    static void generateKey() throws Exception {
        KeyPairGenerator generator = KeyPairGenerator.getInstance("RSA");
        generator.initialize(2048);
        keyPair = generator.generateKeyPair();
        publicKeyPem = "-----BEGIN PUBLIC KEY-----\n" +
                Base64.getMimeEncoder(64, "\n".getBytes(StandardCharsets.US_ASCII))
                        .encodeToString(keyPair.getPublic().getEncoded()) +
                "\n-----END PUBLIC KEY-----\n";
    }

    @Test
    void verifiesCacheAndRejectsBindingTamperAndExpiry() throws Exception {
        OfflineVerifier verifier = verifier();
        Map<String, Object> claims = cacheClaims();
        String token = sign(claims, keyPair.getPrivate());
        OfflineCacheClaims verified = verifier.verifyOfflineToken(token,
                new OfflineExpectation("lic_test", "device", "machine-A"));
        assertEquals("lic_test", verified.licenseNo());

        VerificationException mismatch = assertThrows(VerificationException.class,
                () -> verifier.verifyOfflineToken(token,
                        new OfflineExpectation("lic_test", "device", "machine-B")));
        assertTrue(mismatch.hasCode(VerificationCode.BINDING_MISMATCH));

        int signatureStart = token.indexOf('.') + 1;
        char replacement = token.charAt(signatureStart) == 'A' ? 'B' : 'A';
        String tampered = token.substring(0, signatureStart) + replacement + token.substring(signatureStart + 1);
        VerificationException invalid = assertThrows(VerificationException.class,
                () -> verifier.verifyOfflineToken(tampered,
                        new OfflineExpectation("lic_test", "device", "machine-A")));
        assertTrue(invalid.hasCode(VerificationCode.INVALID_SIGNATURE));

        claims.put("license_expired_at", "2026-08-11T03:00:00Z");
        VerificationException expired = assertThrows(VerificationException.class,
                () -> verifier.verifyOfflineToken(sign(claims, keyPair.getPrivate()),
                        new OfflineExpectation("lic_test", "device", "machine-A")));
        assertTrue(expired.hasCode(VerificationCode.LICENSE_EXPIRED));

		claims.put("license_expired_at", "2026-08-14T03:00:00Z");
		claims.put("offline_until", "2026-08-11T03:00:00Z");
		VerificationException offlineExpired = assertThrows(VerificationException.class,
				() -> verifier.verifyOfflineToken(sign(claims, keyPair.getPrivate()),
						new OfflineExpectation("lic_test", "device", "machine-A")));
		assertTrue(offlineExpired.hasCode(VerificationCode.OFFLINE_WINDOW_EXPIRED));

		claims.put("offline_until", "2026-08-13T03:00:00Z");
		claims.put("issued_at", "2026-08-12T03:10:00Z");
		VerificationException future = assertThrows(VerificationException.class,
				() -> verifier.verifyOfflineToken(sign(claims, keyPair.getPrivate()),
						new OfflineExpectation("lic_test", "device", "machine-A")));
		assertTrue(future.hasCode(VerificationCode.ISSUED_IN_FUTURE));
    }

    @Test
    void verifiesFullOfflineFileAndRejectsWrongMachine() throws Exception {
        Map<String, Object> claims = new LinkedHashMap<>();
        claims.put("version", 1);
        claims.put("license_no", "off_test");
        claims.put("product_id", "prod_test");
        claims.put("product_code", "YN");
        claims.put("product_name", "Test Product");
        claims.put("app_key", "app_test");
        claims.put("bind_mode", "device");
        claims.put("machine_code", "MACHINE-ABC");
        claims.put("issued_at", NOW.toString());
        claims.put("expired_at", "2027-08-12T03:00:00Z");
        claims.put("is_permanent", false);
        String file = MAPPER.writeValueAsString(Map.of(
                "format", "yn-license-key",
                "version", 1,
                "token", sign(claims, keyPair.getPrivate())));
        assertEquals("off_test", verifier().verifyLicenseFile(file, "machine-abc").licenseNo());
        VerificationException mismatch = assertThrows(VerificationException.class,
                () -> verifier().verifyLicenseFile(file, "other-machine"));
        assertTrue(mismatch.hasCode(VerificationCode.BINDING_MISMATCH));

		claims.put("is_permanent", true);
		claims.remove("expired_at");
		String permanentFile = MAPPER.writeValueAsString(Map.of(
				"format", "yn-license-key",
				"version", 1,
				"token", sign(claims, keyPair.getPrivate())));
		assertTrue(verifier().verifyLicenseFile(permanentFile, "machine-abc").permanent());
    }

    private static OfflineVerifier verifier() {
        return OfflineVerifier.builder(publicKeyPem, "YN")
                .clock(Clock.fixed(NOW, ZoneOffset.UTC))
                .clockSkew(Duration.ofMinutes(5))
                .build();
    }

    private static Map<String, Object> cacheClaims() {
        Map<String, Object> claims = new LinkedHashMap<>();
        claims.put("type", "offline_cache");
        claims.put("license_no", "lic_test");
        claims.put("product_code", "YN");
        claims.put("bind_mode", "device");
        claims.put("bind_digest", OfflineVerifier.bindingDigest("device", "machine-A"));
        claims.put("license_expired_at", "2026-08-14T03:00:00Z");
        claims.put("offline_until", "2026-08-13T03:00:00Z");
        claims.put("issued_at", NOW.toString());
        claims.put("key_version", 1);
        return claims;
    }

    @Test
    void keyRingSelectsSigningKeyByVersion() throws Exception {
        KeyPairGenerator generator = KeyPairGenerator.getInstance("RSA");
        generator.initialize(2048);
        KeyPair keyPairV2 = generator.generateKeyPair();
        String publicKeyPemV2 = "-----BEGIN PUBLIC KEY-----\n" +
                Base64.getMimeEncoder(64, "\n".getBytes(StandardCharsets.US_ASCII))
                        .encodeToString(keyPairV2.getPublic().getEncoded()) +
                "\n-----END PUBLIC KEY-----\n";
        Map<Integer, String> ring = Map.of(1, publicKeyPem, 2, publicKeyPemV2);
        OfflineVerifier ringVerifier = OfflineVerifier.Builder.keyRing(ring, "YN")
                .clock(Clock.fixed(NOW, ZoneOffset.UTC))
                .build();

        // v2 签发的新 token（密钥轮换后）与 v1 旧 token 都能通过。
        Map<String, Object> v2Claims = cacheClaims();
        v2Claims.put("license_no", "lic_ring");
        v2Claims.put("key_version", 2);
        String v2Token = sign(v2Claims, keyPairV2.getPrivate());
        assertEquals("lic_ring", ringVerifier.verifyOfflineToken(v2Token,
                new OfflineExpectation("lic_ring", "device", "machine-A")).licenseNo());

        Map<String, Object> v1Claims = cacheClaims();
        v1Claims.put("license_no", "lic_ring");
        String v1Token = sign(v1Claims, keyPair.getPrivate());
        assertEquals("lic_ring", ringVerifier.verifyOfflineToken(v1Token,
                new OfflineExpectation("lic_ring", "device", "machine-A")).licenseNo());

        // 未知版本拒绝。
        Map<String, Object> v9Claims = cacheClaims();
        v9Claims.put("key_version", 9);
        VerificationException unknown = assertThrows(VerificationException.class,
                () -> ringVerifier.verifyOfflineToken(sign(v9Claims, keyPairV2.getPrivate()),
                        new OfflineExpectation("lic_ring", "device", "machine-A")));
        assertTrue(unknown.hasCode(VerificationCode.KEY_VERSION_UNKNOWN));

        // 环外私钥伪造 key_version=2 不能通过（签名校验仍然生效）。
        Map<String, Object> forgedClaims = cacheClaims();
        forgedClaims.put("license_no", "lic_ring");
        forgedClaims.put("key_version", 2);
        KeyPair foreign = generator.generateKeyPair();
        VerificationException forged = assertThrows(VerificationException.class,
                () -> ringVerifier.verifyOfflineToken(sign(forgedClaims, foreign.getPrivate()),
                        new OfflineExpectation("lic_ring", "device", "machine-A")));
        assertTrue(forged.hasCode(VerificationCode.INVALID_SIGNATURE));
    }

    private static String sign(Object claims, PrivateKey privateKey) throws Exception {
        byte[] body = MAPPER.writeValueAsBytes(claims);
        Signature signature = Signature.getInstance("SHA256withRSA");
        signature.initSign(privateKey);
        signature.update(body);
        return Base64.getUrlEncoder().withoutPadding().encodeToString(body) + "." +
                Base64.getUrlEncoder().withoutPadding().encodeToString(signature.sign());
    }
}
