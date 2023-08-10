#!/usr/bin/python3
import asyncio
import json
import os
import sys
import traceback

from azure.iot.device.aio import IoTHubModuleClient
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives import serialization


async def main():
    cfg = None
    with open("/etc/mender/mender-connect.conf") as f:
        cfg = json.load(f)

    if cfg is None:
        return

    auth_cfg = cfg.get("Authentication")
    if auth_cfg is None or auth_cfg.get("type", "").lower() == "dbus":
        return

    update_conf = False
    tenant_token = os.environ.get("TENANT_TOKEN")
    if tenant_token is not None and auth_cfg.get("TenantToken") is None:
        auth_cfg["TenantToken"] = tenant_token
        update_conf = True
    external_id = os.environ.get("IOTEDGE_DEVICEID")
    if external_id is not None:
        id_data = auth_cfg.get("IdentityData")
        hub_name = os.environ.get("IOTEDGE_IOTHUBHOSTNAME", "iot-edge").split(".")[0]
        id_data[f"{hub_name}:device_id"] = external_id
        module_id = os.environ.get("IOTEDGE_MODULEID")
        if module_id is not None:
            external_id += f"/{module_id}"
            id_data[f"{hub_name}:module_id"] = module_id
        auth_cfg["ExternalID"] = "iot-hub " + external_id
        auth_cfg["IdentityData"] = id_data
        update_conf = True

    if update_conf:
        with open("/etc/mender/mender-connect.conf", "w") as f:
            cfg["Authentication"] = auth_cfg
            json.dump(cfg, f, indent=2)

    pkey_path = cfg.get("Authentication", {}).get("PrivateKey")
    if not os.path.exists(pkey_path):
        os.makedirs(os.path.dirname(pkey_path))
        private_key = ec.generate_private_key(ec.SECP256R1())
        pem = private_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        )
        with open(pkey_path, "wb") as f:
            f.write(pem)
    else:
        with open(pkey_path, "rb") as f:
            pem = f.read()
            private_key = serialization.load_pem_private_key(pem, password=None)

    await client.patch_twin_reported_properties(
        {
            "pubkey": private_key.public_key()
            .public_bytes(
                encoding=serialization.Encoding.PEM,
                format=serialization.PublicFormat.SubjectPublicKeyInfo,
            )
            .decode("ascii"),
            "id_data": auth_cfg.get("IdentityData", {}),
        }
    )


if __name__ == "__main__":
    try:
        client = IoTHubModuleClient.create_from_edge_environment()
        print("bootstrapping...")
        asyncio.run(main())
    except Exception as e:
        traceback.print_exc()
        print(e)
    finally:
        os.execv("/usr/bin/nt-connect", ["nt-connect"] + sys.argv[1:])
