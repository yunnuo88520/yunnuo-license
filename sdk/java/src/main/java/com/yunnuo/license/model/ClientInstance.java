package com.yunnuo.license.model;
import com.fasterxml.jackson.annotation.JsonProperty;
import java.util.Map;

/** Per-installation credentials, never the shared developer AppSecret. */
public record ClientInstance(
 @JsonProperty("instance_id") String instanceId,
 @JsonProperty("instance_secret") String instanceSecret,
 @JsonProperty("device_id") String deviceId,
 @JsonProperty("seat_id") String seatId,
 @JsonProperty("bindings") Map<String,String> bindings,
 @JsonProperty("target") Map<String,Object> target,
 @JsonProperty("current_version") String currentVersion,
 @JsonProperty("session_id") String sessionId) {
 @Override public String toString(){return "ClientInstance[id="+instanceId+", credential=redacted]";}
 public ClientInstance(String id,String secret,String device){this(id,secret,device,null,null,null,null,null);}
 public ClientInstance withSession(String value){return new ClientInstance(instanceId,instanceSecret,deviceId,seatId,bindings,target,currentVersion,value);}
 public ClientInstance withSeat(String id){return new ClientInstance(instanceId,instanceSecret,deviceId,id,bindings,target,currentVersion,sessionId);}
 public ClientInstance withTarget(Map<String,Object> value,String version){return new ClientInstance(instanceId,instanceSecret,deviceId,seatId,bindings,value,version,sessionId);}
}
