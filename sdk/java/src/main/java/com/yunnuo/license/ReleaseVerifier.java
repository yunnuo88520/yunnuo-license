package com.yunnuo.license;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.NullNode;
import java.nio.charset.StandardCharsets;
import java.security.KeyFactory;
import java.security.MessageDigest;
import java.security.Signature;
import java.security.spec.X509EncodedKeySpec;
import java.util.Base64;
import java.util.HexFormat;

/** Signature, product, complete target metadata, size and hash are all checked. */
public final class ReleaseVerifier {
 private ReleaseVerifier(){}
 public static JsonNode verifyManifest(JsonNode release,String pem,String productId){
  try {
   String[] parts=release.path("manifest_token").asText().split("\\.",-1);
   if(parts.length!=2)throw new LicenseSDKException("ynlicense: invalid release manifest");
   byte[] raw=Base64.getUrlDecoder().decode(parts[0]);
   String clean=pem.replace("-----BEGIN PUBLIC KEY-----","").replace("-----END PUBLIC KEY-----","").replaceAll("\\s","");
   var key=KeyFactory.getInstance("RSA").generatePublic(new X509EncodedKeySpec(Base64.getDecoder().decode(clean)));
   var verifier=Signature.getInstance("SHA256withRSA");verifier.initVerify(key);verifier.update(raw);
   if(!verifier.verify(Base64.getUrlDecoder().decode(parts[1])))throw new LicenseSDKException("ynlicense: invalid release signature");
   JsonNode claims=LicenseClient.defaultObjectMapper().readTree(raw);
   if(productId==null||productId.isBlank()||!"product_release".equals(claims.path("type").asText())||!productId.equals(claims.path("product_id").asText()))throw new LicenseSDKException("ynlicense: wrong release product");
   for(String field:new String[]{"product_id","version","size","sha256","key_version"})
    if(!claims.path(field).equals(release.path(field)))throw new LicenseSDKException("ynlicense: signed release metadata mismatch");
   if(!claims.path("release_id").asText().equals(release.path("id").asText())||claims.path("size").asLong()<=0||claims.path("size").asLong()>256L*1024*1024)throw new LicenseSDKException("ynlicense: invalid release size or identity");
   JsonNode a=claims.path("target"),b=release.path("target");if(a.isMissingNode())a=NullNode.instance;if(b.isMissingNode())b=NullNode.instance;
   if(!a.equals(b))throw new LicenseSDKException("ynlicense: signed release target mismatch");
   return claims;
  }catch(LicenseSDKException e){throw e;}catch(Exception e){throw new LicenseSDKException("ynlicense: invalid release manifest",e);}
 }
 public static void verifyTarget(JsonNode release,com.yunnuo.license.model.ClientInstance instance){
  JsonNode target=release.path("target");if(target.isMissingNode()||target.isNull())return;
  if(instance==null||instance.target()==null)throw new LicenseSDKException("ynlicense: explicit client target required");
  var client=instance.target();
  for(String field:new String[]{"os","arch","runtime"})if(!"any".equals(target.path(field).asText())&&!target.path(field).asText().equals(client.get(field)))throw new LicenseSDKException("ynlicense: wrong client platform");
  if(!target.path("channel").asText().equals(client.getOrDefault("channel","stable"))||client.containsKey("kind")&&!target.path("kind").asText().equals(client.get("kind")))throw new LicenseSDKException("ynlicense: wrong channel or package kind");
  String min=target.path("min_version").asText(),max=target.path("max_version").asText(),version=instance.currentVersion();
  if(!min.isBlank()||!max.isBlank()){
   if(version==null||!version.matches("(0|[1-9][0-9]{0,8})[.](0|[1-9][0-9]{0,8})[.](0|[1-9][0-9]{0,8})")||!min.isBlank()&&compare(version,min)<0||!max.isBlank()&&compare(version,max)>0)throw new LicenseSDKException("ynlicense: source version outside signed upgrade path");
  }
 }
 private static int compare(String a,String b){String[] x=a.split("[.]"),y=b.split("[.]");for(int i=0;i<3;i++){int c=Integer.compare(Integer.parseInt(x[i]),Integer.parseInt(y[i]));if(c!=0)return c;}return 0;}
 public static void verifyPackage(JsonNode release,byte[] content,String pem,String productId){
  JsonNode claims=verifyManifest(release,pem,productId);
  try{
   String hash=HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(content));
   if(content.length!=claims.path("size").asLong()||!hash.equals(claims.path("sha256").asText()))throw new LicenseSDKException("ynlicense: release checksum mismatch");
  }catch(LicenseSDKException e){throw e;}catch(Exception e){throw new LicenseSDKException("ynlicense: checksum failed",e);}
 }
}
