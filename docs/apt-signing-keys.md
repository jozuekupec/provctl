# APT signing keys

This runbook implements the signing-key requirements in
[specification §26](project-specification.md#26-apt-repozitář-na-github-pages).
Run commands as your regular user on your workstation, not in the disposable
`pv` container. Never commit private keys, passphrases, or revocation certificates.
Only `packaging/apt/provctl.asc` is public repository content.

## Create the repository identity

Prerequisites: GnuPG and authenticated GitHub CLI (`gh auth status`; use
`gh auth login` if needed). SSH access for Git does not authenticate GitHub CLI.
If the keys already exist, skip creation and find their fingerprints below.

```bash
gpg --quick-generate-key "provctl APT repository" rsa4096 cert 2y
gpg --list-secret-keys --with-subkey-fingerprint "provctl APT repository"
```

Choose a strong passphrase and store it in your encrypted vault. Set the full
primary fingerprint shown beneath `sec`, then create a signing-only subkey:

```bash
APT_PRIMARY_FPR='REPLACE_WITH_PRIMARY_FINGERPRINT'
gpg --quick-add-key "$APT_PRIMARY_FPR" rsa4096 sign 1y
gpg --list-secret-keys --with-subkey-fingerprint "$APT_PRIMARY_FPR"
APT_SIGNING_FPR='REPLACE_WITH_SIGNING_SUBKEY_FINGERPRINT'
```

The signing fingerprint is beneath `ssb` with capability `[S]`. Record both
fingerprints and expiration dates in the vault. The primary key manages the
identity; CI receives only the signing subkey.

## Export the public key and configure GitHub

From the repository root:

```bash
mkdir -p packaging/apt
gpg --armor --output packaging/apt/provctl.asc --export "$APT_PRIMARY_FPR"
set -o pipefail
gpg --armor --export-secret-subkeys "${APT_SIGNING_FPR}!" |
  gh secret set APT_GPG_PRIVATE_KEY --repo jozuekupec/provctl
gh secret set APT_GPG_PASSPHRASE --repo jozuekupec/provctl
gh secret list --repo jozuekupec/provctl
```

Enter the subkey passphrase at the interactive secret prompt. Do not place it
in command arguments or chat. The final command checks secret names, not their
contents or whether signing works. A release signing check must verify that.
The `!` selects the exact subkey; `--export-secret-subkeys` excludes the usable
private primary key. If updating an existing public export, review GnuPG's
overwrite prompt and the resulting public-key diff.

## Back up to an encrypted vault

Yes: back up the full private key, including the primary key, for recovery and
subkey renewal. GitHub Secrets are deployment credentials, not a recoverable
backup. Keep the full private export out of GitHub Secrets as well as Git.

Create a private staging directory outside the repository:

```bash
umask 077
APT_BACKUP_DIR=$(mktemp -d /tmp/provctl-apt-key-backup.XXXXXX)
gpg --armor --output "$APT_BACKUP_DIR/private-full.asc" --export-secret-keys "$APT_PRIMARY_FPR"
gpg --armor --output "$APT_BACKUP_DIR/public.asc" --export "$APT_PRIMARY_FPR"
gpg --armor --output "$APT_BACKUP_DIR/revoke.asc" --gen-revoke "$APT_PRIMARY_FPR"
```

Generating the revocation certificate does not revoke the key. Importing and
publishing it does; retain it for loss or compromise, and do not import it
during an ordinary restore.

Upload these three files as encrypted vault attachments. Also record:

- Primary and signing-subkey fingerprints, identity, and expiration dates.
- The passphrase needed to unlock the private key.
- Repository `jozuekupec/provctl` and the two GitHub secret names above.
- A link or copy of this runbook and a reminder before either key expires.

Verify that attachments can be downloaded and the private key can be restored
before deleting the staging files. Delete only those exact files and their
staging directory after verification; ordinary deletion is not guaranteed
secure erasure on SSDs. Use encrypted storage for sensitive backups and an
independently recoverable vault backup.

## Verify recovery

Download `private-full.asc` from the vault to a private directory. Import into
a separate temporary keyring, leaving your normal keyring unchanged:

```bash
APT_RECOVERY_DIR=$(mktemp -d /tmp/provctl-apt-key-recovery.XXXXXX)
gpg --homedir "$APT_RECOVERY_DIR" --import /absolute/path/to/private-full.asc
gpg --homedir "$APT_RECOVERY_DIR" --list-secret-keys --with-subkey-fingerprint
printf 'provctl APT signing recovery test\n' > "$APT_RECOVERY_DIR/message.txt"
gpg --homedir "$APT_RECOVERY_DIR" --local-user "${APT_SIGNING_FPR}!" --armor --detach-sign "$APT_RECOVERY_DIR/message.txt"
gpg --homedir "$APT_RECOVERY_DIR" --verify "$APT_RECOVERY_DIR/message.txt.asc" "$APT_RECOVERY_DIR/message.txt"
```

Check both fingerprints against the vault record and confirm a good signature.
The signing command must unlock with the saved passphrase. The temporary
keyring contains private keys: clean up that exact directory after verification.
For actual recovery, import the backup into your intended GnuPG keyring and
repeat the GitHub subkey export above if credentials need replacement.

## Expiration and compromise

Before expiration, use the primary private key to extend the appropriate key's
validity or create a replacement signing subkey. Re-export the public key and
CI subkey, update the signing fingerprint when it changes, publish updated
public-key material, and verify APT clients receive it. Extending a local key
alone does not update CI or installed client keyrings.

For compromise, stop publishing, revoke the affected key/subkey, rotate CI
credentials, and distribute the replacement trust material to clients. Do not
publish the primary revocation certificate merely to renew an expired subkey.

## References

- [GnuPG key management](https://gnupg.org/documentation/manuals/gnupg/OpenPGP-Key-Management.html)
- [GnuPG export and revocation commands](https://gnupg.org/documentation/manuals/gnupg/Operational-GPG-Commands.html)
- [GitHub CLI secret storage](https://cli.github.com/manual/gh_secret_set)
