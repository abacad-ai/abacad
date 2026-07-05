package dev.abacad.probe

import android.app.admin.DeviceAdminReceiver

/**
 * Device-admin receiver whose ONLY purpose is to unlock `DevicePolicyManager.lockNow()`,
 * which the `sleep` command uses to turn the screen off between tasks. The policy set
 * (see res/xml/device_admin.xml) is limited to force-lock — nothing wipe-related.
 *
 * Enabling this is a one-time human step during setup (MainActivity button). Without it,
 * `sleep` is a no-op and the screen just follows the normal display timeout instead.
 */
class AbacadDeviceAdmin : DeviceAdminReceiver()
