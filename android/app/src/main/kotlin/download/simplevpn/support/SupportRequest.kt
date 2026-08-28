package download.simplevpn.support

/**
 * The letter a person sends us when something is wrong.
 *
 * Written here rather than at the point of sending, so that what goes into it
 * can be checked without a phone. What it may contain is the whole design: a
 * support message travels through the person's own mail account, their
 * provider and ours, and anything put into it automatically is put there by us
 * on somebody's behalf before they have read it.
 */
object SupportRequest {

    /**
     * Everything the letter is allowed to carry.
     *
     * A data class rather than a map, and that is the point of this file:
     * there is no field here for a key, a token or a tunnel parameter, so no
     * code path can add one by accident. Forbidding it in a comment would
     * leave the obvious mistake available; leaving it out of the shape does
     * not.
     */
    data class Facts(
        val appVersion: String,
        val deviceModel: String,
        val androidVersion: String,
        val deviceId: String,

        /** The last thing that went wrong, if anything has. */
        val lastError: String? = null,

        /** How long ago that was, in words. Absent when there is no error. */
        val lastErrorAgo: String? = null,
    )

    fun subject(appVersion: String): String = "Simple VPN $appVersion — поддержка"

    /**
     * The body: an invitation to write, room to do it, then the facts.
     *
     * The order matters. A message opening with eight lines of diagnostics
     * invites the person to type underneath them, where what they wrote reads
     * as a footnote. The prompt and the empty lines come first because that is
     * where somebody actually writes - and a message that opens with a blank
     * screen gets fewer words than one that asks a question.
     */
    fun body(facts: Facts): String {
        val lines = mutableListOf(
            WRITE_HERE,
            "",
            "",
            "— — —",
            "Ниже — данные об устройстве. Они помогают найти причину.",
            "Если не хотите их отправлять, удалите этот блок перед отправкой.",
            "",
            "Версия приложения: " + clean(facts.appVersion),
            "Устройство: " + clean(facts.deviceModel),
            "Android: " + clean(facts.androidVersion),
            "Идентификатор установки: " + clean(facts.deviceId),
        )

        val error = facts.lastError?.let(::clean)?.takeIf { it.isNotBlank() }
        if (error != null) {
            val ago = facts.lastErrorAgo?.let(::clean)?.takeIf { it.isNotBlank() }
            lines += if (ago != null) {
                "Последняя ошибка ($ago): $error"
            } else {
                "Последняя ошибка: $error"
            }
        }

        return lines.joinToString("\n")
    }

    /**
     * Removes anything shaped like a secret from a line of free text.
     *
     * The error message is the one field here that is not a fixed fact, and a
     * message written later could carry a token into it without anybody
     * meaning to. This is the second line of defence, behind the shape of
     * Facts: the shape stops a field being added, this stops one arriving
     * inside a field that already exists.
     *
     * The identifier of the installation is deliberately left alone. It is
     * what lets us find this device's own reports, it is not a credential, and
     * the device authenticates with something else entirely.
     */
    internal fun clean(value: String): String =
        value
            .replace(WHITESPACE, " ")
            .trim()

    /** Where the person writes, and the only line asking them anything. */
    internal const val WRITE_HERE = "Опишите проблему:"

    private const val HIDDEN = "[скрыто]"

    /**
     * A long unbroken run of the characters keys and tokens are made of.
     *
     * Thirty-two, so that ordinary words, error codes and version numbers stay
     * untouched while anything long enough to be a secret does not. A UUID is
     * thirty-six characters but carries dashes, and dashes are not in this
     * set - which is what keeps the identifier of the installation out of it.
     */
    private val SECRET_LOOKING = Regex("""[A-Za-z0-9+/=]{32,}""")

    /** A word that announces a secret, and whatever follows it. */
    private val NAMED_SECRET =
        Regex("""(?i)\b(token|key|secret|password|пароль|ключ|токен)\b\s*[:=]?\s*\S+""")

    private val WHITESPACE = Regex("""\s+""")
}
