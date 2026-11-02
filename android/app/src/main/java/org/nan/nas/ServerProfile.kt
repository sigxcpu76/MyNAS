package org.nan.nas

import java.io.Serializable

data class ServerProfile(
    val protocol: String = "https",
    val host: String = "",
    val port: Int = 8443,
    val username: String = "",
    val token: String = "",
    val trustSelfSigned: Boolean = true,
    val remember: Boolean = true
) : Serializable {
    val baseUrl: String
        get() = "$protocol://$host:$port"
}
