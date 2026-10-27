package com.yunnuo.license.model;
public record OfflineExpectation(String licenseNo,String bindMode,String bindValue,String instanceId,String seatId,String sessionId){
 public OfflineExpectation(String no,String mode,String value){this(no,mode,value,null,null,null);}
 public OfflineExpectation(String no,String mode,String value,String instance,String seat){this(no,mode,value,instance,seat,null);}
}
