package com.yunnuo.license;
import com.yunnuo.license.model.ClientInstance;
import java.nio.file.*;
import java.nio.file.attribute.PosixFilePermissions;
import java.security.SecureRandom;
import java.util.HexFormat;

/** Caller supplies a private persistent directory, outside public web assets. */
public final class InstallationIdentity {
 private InstallationIdentity(){}
 public static ClientInstance open(Path dir,String device){
  try{
   Files.createDirectories(dir);Path path=dir.resolve("instance.json");
   if(!Files.exists(path,LinkOption.NOFOLLOW_LINKS)){
    byte[] bytes=new byte[64];new SecureRandom().nextBytes(bytes);var hex=HexFormat.of();
    ClientInstance created=new ClientInstance(hex.formatHex(bytes,0,16),hex.formatHex(bytes,16,48),device==null||device.isBlank()?hex.formatHex(bytes,48,64):device);
    Path temp=Files.createTempFile(dir,".identity-",".tmp");
    try{
     try{Files.setPosixFilePermissions(temp,PosixFilePermissions.fromString("rw-------"));}catch(UnsupportedOperationException ignored){}
     Files.write(temp,LicenseClient.defaultObjectMapper().writeValueAsBytes(created));
     try{Files.createLink(path,temp);}catch(FileAlreadyExistsException ignored){}
    }finally{Files.deleteIfExists(temp);}
   }
   if(!Files.isRegularFile(path,LinkOption.NOFOLLOW_LINKS)||Files.size(path)>8192)throw new LicenseSDKException("ynlicense: invalid installation file");
   ClientInstance value=LicenseClient.defaultObjectMapper().readValue(Files.readAllBytes(path),ClientInstance.class);
   if(value.instanceId()==null||!value.instanceId().matches("[a-z0-9_-]{16,64}")||value.instanceSecret()==null||!value.instanceSecret().matches("[a-f0-9]{64}"))throw new LicenseSDKException("ynlicense: invalid installation identity");
   if(device!=null&&!device.isBlank()&&!device.equals(value.deviceId()))throw new LicenseSDKException("ynlicense: configured device changed; use controlled rebind");
   return value;
  }catch(LicenseSDKException e){throw e;}catch(Exception e){throw new LicenseSDKException("ynlicense: installation storage failed",e);}
 }
}
