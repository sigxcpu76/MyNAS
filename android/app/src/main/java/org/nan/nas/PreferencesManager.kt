package org.nan.nas

import android.content.Context
import android.content.SharedPreferences

class PreferencesManager(context: Context) {
    private val prefs: SharedPreferences = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    companion object {
        private const val PREFS_NAME = "nan_nas_preferences"
        private const val KEY_PROTOCOL = "server_protocol"
        private const val KEY_HOST = "server_host"
        private const val KEY_PORT = "server_port"
        private const val KEY_USERNAME = "server_username"
        private const val KEY_TOKEN = "server_token"
        private const val KEY_TRUST_SSL = "trust_self_signed_ssl"
        private const val KEY_REMEMBER = "remember_server"
    }

    fun saveProfile(profile: ServerProfile) {
        prefs.edit().apply {
            putString(KEY_PROTOCOL, profile.protocol)
            putString(KEY_HOST, profile.host)
            putInt(KEY_PORT, profile.port)
            putString(KEY_USERNAME, profile.username)
            putString(KEY_TOKEN, profile.token)
            putBoolean(KEY_TRUST_SSL, profile.trustSelfSigned)
            putBoolean(KEY_REMEMBER, profile.remember)
            apply()
        }
    }

    fun getProfile(): ServerProfile? {
        val host = prefs.getString(KEY_HOST, null) ?: return null
        if (host.isEmpty()) return null

        return ServerProfile(
            protocol = prefs.getString(KEY_PROTOCOL, "https") ?: "https",
            host = host,
            port = prefs.getInt(KEY_PORT, 8443),
            username = prefs.getString(KEY_USERNAME, "") ?: "",
            token = prefs.getString(KEY_TOKEN, "") ?: "",
            trustSelfSigned = prefs.getBoolean(KEY_TRUST_SSL, true),
            remember = prefs.getBoolean(KEY_REMEMBER, true)
        )
    }

    fun clearToken() {
        prefs.edit().remove(KEY_TOKEN).apply()
    }

    fun clearAll() {
        prefs.edit().clear().apply()
    }
}
