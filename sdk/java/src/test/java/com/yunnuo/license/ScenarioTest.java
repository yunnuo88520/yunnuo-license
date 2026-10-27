package com.yunnuo.license;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;
import java.nio.file.Path;
import java.util.Map;
import com.yunnuo.license.model.*;
import static org.junit.jupiter.api.Assertions.*;

class ScenarioTest {
 @TempDir Path temporary;
 @Test void equivalentIPv6Spelling(){assertEquals(OfflineVerifier.bindingDigest("ip","2001:db8::40"),OfflineVerifier.bindingDigest("ip","2001:0DB8:0000:0000:0000:0000:0000:0040"));}
 @Test void persistentIdentityAndActualHost(){
  var first=InstallationIdentity.open(temporary,"device-provider-A");var second=InstallationIdentity.open(temporary,"device-provider-A");
  assertEquals(first.instanceId(),second.instanceId());assertEquals(first.instanceSecret(),second.instanceSecret());
  assertFalse(first.toString().contains(first.instanceSecret()));
  assertThrows(LicenseSDKException.class,()->InstallationIdentity.open(temporary,"other-device"));
  var input=new VerifyRequest("lic","domain_wildcard","*.example.com",null);
  assertEquals("other.example.net",WebBinding.forHost(input,"other.example.net:443").bindValue());
  assertThrows(LicenseSDKException.class,()->WebBinding.forHost(input,"user@allowed.example.com"));
 }
 @Test void validSignatureMetadataStillNeedsClientPlatformAndUpgradePath()throws Exception{
  var release=LicenseClient.defaultObjectMapper().readTree("{\"target\":{\"os\":\"linux\",\"arch\":\"amd64\",\"runtime\":\"jvm\",\"kind\":\"archive\",\"channel\":\"stable\",\"min_version\":\"1.0.0\",\"max_version\":\"1.9.9\"}}");
  var input=new ClientInstance("instance_00000001","secret","device").withTarget(Map.of("os","linux","arch","amd64","runtime","jvm","kind","archive","channel","stable"),"1.0.0");
  assertDoesNotThrow(()->ReleaseVerifier.verifyTarget(release,input));
  assertThrows(LicenseSDKException.class,()->ReleaseVerifier.verifyTarget(release,input.withTarget(input.target(),"0.9.0")));
  assertThrows(LicenseSDKException.class,()->ReleaseVerifier.verifyTarget(release,input.withTarget(Map.of("os","windows"),"1.0.0")));
 }
}
