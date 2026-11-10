package org.nan.nas

import android.content.Context
import android.content.SharedPreferences
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.nio.charset.StandardCharsets
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

class PreferencesManager(context: Context) {
    private val prefs: SharedPreferences = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    companion object {
        private const val PREFS_NAME = "nan_nas_preferences"
        private const val KEYSTORE_PROVIDER = "AndroidKeyStore"
        private const val KEY_ALIAS = "nan_nas_prefs_secret_key"
        private const val ENC_PREFIX = "enc:v1:"
        private const val OBF_PREFIX = "obf:v1:"
        private val OBF_MASK = byteArrayOf(0x4E, 0x61, 0x4E, 0x2D, 0x4E, 0x41, 0x53, 0x5F, 0x32, 0x36)

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
            putString(KEY_USERNAME, encodeSecret(profile.username))
            putString(KEY_TOKEN, encodeSecret(profile.token))
            putBoolean(KEY_TRUST_SSL, profile.trustSelfSigned)
            putBoolean(KEY_REMEMBER, profile.remember)
            apply()
        }
    }

    fun getProfile(): ServerProfile? {
        val host = prefs.getString(KEY_HOST, null) ?: return null
        if (host.isEmpty()) return null

        val rawUsername = prefs.getString(KEY_USERNAME, "") ?: ""
        val rawToken = prefs.getString(KEY_TOKEN, "") ?: ""
        val decodedUsername = decodeSecret(rawUsername)
        val decodedToken = decodeSecret(rawToken)

        // Automatically migrate legacy cleartext values if present
        if ((rawToken.isNotEmpty() && !isEncoded(rawToken)) ||
            (rawUsername.isNotEmpty() && !isEncoded(rawUsername))
        ) {
            prefs.edit().apply {
                if (decodedUsername.isNotEmpty()) putString(KEY_USERNAME, encodeSecret(decodedUsername))
                if (decodedToken.isNotEmpty()) putString(KEY_TOKEN, encodeSecret(decodedToken))
                apply()
            }
        }

        return ServerProfile(
            protocol = prefs.getString(KEY_PROTOCOL, "https") ?: "https",
            host = host,
            port = prefs.getInt(KEY_PORT, 8443),
            username = decodedUsername,
            token = decodedToken,
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

    private fun isEncoded(value: String): Boolean {
        return value.startsWith(ENC_PREFIX) || value.startsWith(OBF_PREFIX)
    }

    private fun getOrCreateSecretKey(): SecretKey {
        val keyStore = KeyStore.getInstance(KEYSTORE_PROVIDER).apply { load(null) }
        val existingKey = keyStore.getEntry(KEY_ALIAS, null) as? KeyStore.SecretKeyEntry
        if (existingKey != null) {
            return existingKey.secretKey
        }
        val keyGenerator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE_PROVIDER)
        val spec = KeyGenParameterSpec.Builder(
            KEY_ALIAS,
            KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT
        )
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setKeySize(256)
            .build()
        keyGenerator.init(spec)
        return keyGenerator.generateKey()
    }

    private fun encodeSecret(plainText: String): String {
        if (plainText.isEmpty()) return ""
        return try {
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.ENCRYPT_MODE, getOrCreateSecretKey())
            val iv = Base64.encodeToString(cipher.iv, Base64.NO_WRAP)
            val encrypted = Base64.encodeToString(
                cipher.doFinal(plainText.toByteArray(StandardCharsets.UTF_8)),
                Base64.NO_WRAP
            )
            "$ENC_PREFIX$iv:$encrypted"
        } catch (e: Exception) {
            val bytes = plainText.toByteArray(StandardCharsets.UTF_8)
            val masked = ByteArray(bytes.size) { i ->
                (bytes[i].toInt() xor OBF_MASK[i % OBF_MASK.size].toInt()).toByte()
            }
            OBF_PREFIX + Base64.encodeToString(masked, Base64.NO_WRAP)
        }
    }

    private fun decodeSecret(storedValue: String): String {
        if (storedValue.isEmpty()) return ""
        if (storedValue.startsWith(ENC_PREFIX)) {
            return try {
                val payload = storedValue.removePrefix(ENC_PREFIX)
                val parts = payload.split(":", limit = 2)
                if (parts.size != 2) return ""
                val iv = Base64.decode(parts[0], Base64.NO_WRAP)
                val cipherBytes = Base64.decode(parts[1], Base64.NO_WRAP)
                val cipher = Cipher.getInstance("AES/GCM/NoPadding")
                cipher.init(Cipher.DECRYPT_MODE, getOrCreateSecretKey(), GCMParameterSpec(128, iv))
                String(cipher.doFinal(cipherBytes), StandardCharsets.UTF_8)
            } catch (e: Exception) {
                ""
            }
        }
        if (storedValue.startsWith(OBF_PREFIX)) {
            return try {
                val payload = storedValue.removePrefix(OBF_PREFIX)
                val masked = Base64.decode(payload, Base64.NO_WRAP)
                val unmasked = ByteArray(masked.size) { i ->
                    (masked[i].toInt() xor OBF_MASK[i % OBF_MASK.size].toInt()).toByte()
                }
                String(unmasked, StandardCharsets.UTF_8)
            } catch (e: Exception) {
                ""
            }
        }
        return storedValue
    }
}
