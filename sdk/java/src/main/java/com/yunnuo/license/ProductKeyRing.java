package com.yunnuo.license;

import java.time.Instant;
import java.util.List;

/** /v1/products/keys 返回的产品公钥环，含全部历史版本。 */
public record ProductKeyRing(String productId, String productCode, String appKey, int currentVersion, List<ProductKey> keys) {

    public record ProductKey(String productId, int keyVersion, String publicKeyPem, Instant createdAt) {
    }
}
