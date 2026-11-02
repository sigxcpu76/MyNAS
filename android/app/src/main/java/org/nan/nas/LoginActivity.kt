package org.nan.nas

import android.content.Intent
import android.os.Bundle
import android.view.View
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import org.nan.nas.databinding.ActivityLoginBinding
import java.security.SecureRandom
import java.security.cert.X509Certificate
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManager
import javax.net.ssl.X509TrustManager

class LoginActivity : AppCompatActivity() {
    private lateinit var binding: ActivityLoginBinding
    private lateinit var prefsManager: PreferencesManager

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        binding = ActivityLoginBinding.inflate(layoutInflater)
        setContentView(binding.root)

        prefsManager = PreferencesManager(this)

        // Check if explicit relogin was requested
        val forceRelogin = intent.getBooleanExtra("force_relogin", false)

        val savedProfile = prefsManager.getProfile()
        if (savedProfile != null && savedProfile.token.isNotEmpty() && !forceRelogin) {
            launchMain(savedProfile)
            return
        }

        populateSavedFields(savedProfile)
        setupListeners()
    }

    private fun populateSavedFields(profile: ServerProfile?) {
        if (profile != null) {
            binding.etHost.setText(profile.host)
            binding.etPort.setText(profile.port.toString())
            binding.etUsername.setText(profile.username)
            binding.switchTrustSsl.isChecked = profile.trustSelfSigned
            binding.cbRemember.isChecked = profile.remember
            if (profile.protocol == "http") {
                binding.rbHttp.isChecked = true
            } else {
                binding.rbHttps.isChecked = true
            }
        } else {
            // Default initial values
            binding.etPort.setText("8443")
            binding.rbHttps.isChecked = true
            binding.switchTrustSsl.isChecked = true
            binding.cbRemember.isChecked = true
        }
    }

    private fun setupListeners() {
        binding.rgProtocol.setOnCheckedChangeListener { _, checkedId ->
            if (checkedId == binding.rbHttps.id) {
                if (binding.etPort.text.toString() == "8080") {
                    binding.etPort.setText("8443")
                }
                binding.switchTrustSsl.isEnabled = true
            } else {
                if (binding.etPort.text.toString() == "8443") {
                    binding.etPort.setText("8080")
                }
                binding.switchTrustSsl.isEnabled = false
            }
        }

        binding.btnConnect.setOnClickListener {
            attemptLogin()
        }
    }

    private fun attemptLogin() {
        val host = binding.etHost.text.toString().trim()
        val portStr = binding.etPort.text.toString().trim()
        val username = binding.etUsername.text.toString().trim()
        val password = binding.etPassword.text.toString()
        val protocol = if (binding.rbHttps.isChecked) "https" else "http"
        val trustSelfSigned = binding.switchTrustSsl.isChecked
        val remember = binding.cbRemember.isChecked

        if (host.isEmpty()) {
            showError(getString(R.string.error_invalid_host))
            return
        }

        val port = portStr.toIntOrNull()
        if (port == null || port !in 1..65535) {
            showError(getString(R.string.error_invalid_port))
            return
        }

        if (username.isEmpty() || password.isEmpty()) {
            showError(getString(R.string.error_empty_creds))
            return
        }

        setLoading(true)
        hideError()

        lifecycleScope.launch {
            try {
                val token = withContext(Dispatchers.IO) {
                    executeLoginRequest(protocol, host, port, username, password, trustSelfSigned)
                }

                val profile = ServerProfile(
                    protocol = protocol,
                    host = host,
                    port = port,
                    username = username,
                    token = if (remember) token else "",
                    trustSelfSigned = trustSelfSigned,
                    remember = remember
                )

                if (remember) {
                    prefsManager.saveProfile(profile)
                }

                launchMain(profile.copy(token = token))
            } catch (e: Exception) {
                setLoading(false)
                showError("Login failed: ${e.localizedMessage ?: e.message}")
            }
        }
    }

    private fun executeLoginRequest(
        protocol: String,
        host: String,
        port: Int,
        user: String,
        pass: String,
        trustSelfSigned: Boolean
    ): String {
        val clientBuilder = OkHttpClient.Builder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .readTimeout(10, TimeUnit.SECONDS)

        if (protocol == "https" && trustSelfSigned) {
            val trustAllCerts = arrayOf<TrustManager>(object : X509TrustManager {
                override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
                override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
                override fun getAcceptedIssuers(): Array<X509Certificate> = arrayOf()
            })

            val sslContext = SSLContext.getInstance("TLS")
            sslContext.init(null, trustAllCerts, SecureRandom())
            clientBuilder.sslSocketFactory(sslContext.socketFactory, trustAllCerts[0] as X509TrustManager)
            clientBuilder.hostnameVerifier { _, _ -> true }
        }

        val client = clientBuilder.build()

        val jsonBody = JSONObject().apply {
            put("username", user)
            put("password", pass)
        }

        val body = jsonBody.toString().toRequestBody("application/json; charset=utf-8".toMediaType())
        val url = "$protocol://$host:$port/v1/auth:login"

        val request = Request.Builder()
            .url(url)
            .post(body)
            .build()

        val response = client.newCall(request).execute()
        val responseBody = response.body?.string() ?: ""

        if (!response.isSuccessful) {
            val errMsg = try {
                val json = JSONObject(responseBody)
                json.optString("message", json.optString("error", "HTTP ${response.code}"))
            } catch (e: Exception) {
                "Server responded with status ${response.code}"
            }
            throw IllegalStateException(errMsg)
        }

        val json = JSONObject(responseBody)
        val token = json.optString("token", json.optString("accessToken", json.optString("access_token", "")))
        if (token.isEmpty()) {
            throw IllegalStateException("Server did not return an access token")
        }
        return token
    }

    private fun launchMain(profile: ServerProfile) {
        val intent = Intent(this, MainActivity::class.java).apply {
            putExtra("server_profile", profile)
        }
        startActivity(intent)
        finish()
    }

    private fun setLoading(loading: Boolean) {
        binding.progressBar.visibility = if (loading) View.VISIBLE else View.GONE
        binding.btnConnect.isEnabled = !loading
        binding.etHost.isEnabled = !loading
        binding.etPort.isEnabled = !loading
        binding.etUsername.isEnabled = !loading
        binding.etPassword.isEnabled = !loading
    }

    private fun showError(msg: String) {
        binding.tvError.text = msg
        binding.tvError.visibility = View.VISIBLE
    }

    private fun hideError() {
        binding.tvError.visibility = View.GONE
    }
}
