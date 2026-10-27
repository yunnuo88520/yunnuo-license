package com.yunnuo.license;
import com.yunnuo.license.model.VerifyRequest;
import java.net.URI;
import java.util.Locale;
import java.util.TreeMap;

public final class WebBinding {
 private WebBinding(){}
 /** Pass the actual request Host. Forwarded headers need explicit proxy trust. */
 public static VerifyRequest forHost(VerifyRequest input,String host){
  if(!java.util.Set.of("domain","domain_wildcard","composite").contains(input.bindMode()))return input;
  if(host==null||host.isBlank()||host.matches(".*[\\s,/@?#\\\\].*"))throw new LicenseSDKException("ynlicense: invalid actual request host");
  try{
   URI uri=URI.create("http://"+host);String value=uri.getHost();
   if(value==null||uri.getUserInfo()!=null||!uri.getRawPath().isEmpty())throw new LicenseSDKException("ynlicense: invalid request host");
   value=value.toLowerCase(Locale.ROOT).replaceFirst("\\.$","");
   if("composite".equals(input.bindMode())){
    @SuppressWarnings("unchecked") var map=LicenseClient.defaultObjectMapper().readValue(input.bindValue(),TreeMap.class);
    if(map.containsKey("domain"))map.put("domain",value);if(map.containsKey("domain_wildcard"))map.put("domain_wildcard",value);
    value=LicenseClient.defaultObjectMapper().writeValueAsString(map);
   }
   return new VerifyRequest(input.licenseNo(),input.bindMode(),value,input.licenseToken());
  }catch(LicenseSDKException e){throw e;}catch(Exception e){throw new LicenseSDKException("ynlicense: invalid actual request host",e);}
 }
}
