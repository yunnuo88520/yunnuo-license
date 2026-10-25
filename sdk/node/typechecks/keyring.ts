import { LicenseClient, OfflineVerifier, offlineVerifierFromKeyRing } from "../src/index.js";
const client = new LicenseClient({baseUrl:"https://license.example.com",appKey:"app_example"});
const ring = await client.productKeys();
const keys: Record<number,string> = Object.fromEntries(ring.keys.map(key=>[key.key_version,key.public_key_pem]));
const verifier = new OfflineVerifier({publicKeys:keys,productCode:ring.product_code});
verifier.verifyOfflineToken("signed-token",{licenseNo:"lic_example",bindMode:"domain",bindValue:"ebox.example.com"});
await offlineVerifierFromKeyRing(client,{appKey:"app_example",productCode:"EBOX"});
