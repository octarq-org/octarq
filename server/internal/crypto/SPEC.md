# Engineering Specification — internal/crypto

## 1. Overview
`internal/crypto` provides AES-256-GCM envelope encryption for secrets at rest and constant-time HMAC-SHA256 signing for session tokens and cookies.

## 2. Key Architecture
- **Envelope Pattern**: Bulk application secrets (database passwords, webhook secrets, API keys) are sealed under a random 32-byte Data Encryption Key (DEK).
- **Key Encryption Key (KEK)**: The KEK is derived via SHA-256 from `OCTARQ_SECRET_KEY`.
- **Key Wrapping**: The DEK is encrypted under the KEK using AES-256-GCM and persisted in the settings table at key `crypto.dek`.
- **Key Rotation**: Rotating the master secret `OCTARQ_SECRET_KEY` re-wraps the single DEK; bulk database rows remain untouched.
- **Purpose-Derived Subkeys**: Every other consumer of the master secret names its job and gets its own key from `DeriveKey(secret, purpose)` (HKDF-SHA256, the purpose as info): `octarq/cookie-mac/v1` (session signing), `octarq/csrf/v1`, `octarq/twofa-challenge/v1`, `octarq/export-token/v1`, `octarq/storage-url/v1`, `octarq/download-token/v1`. No two jobs share key material, so a signature oracle in one use never yields the key another use trusts.
- **Session Signing**: `Sign` and `Verify` operate under the `octarq/cookie-mac/v1` subkey, independently of DEK initialization and of the KEK.
- **Why the KEK stays unlabelled**: the wrapped DEK already stored in the settings table was sealed under `SHA-256(OCTARQ_SECRET_KEY)`. Re-deriving the KEK would make every saved secret unreadable, so the KEK keeps its derivation and is left as that derivation's only consumer.

## 3. Failure Modes & Invariants
- **Uninitialized Envelope**: Invoking `Encrypt` or `Decrypt` prior to `EnableEnvelope` fails closed with error `"crypto: envelope not initialized"`.
- **Ciphertext Integrity**: Tampered ciphertexts or truncated data return errors without panicking.
- **Constant-Time Verification**: `Verify` uses `hmac.Equal` to prevent timing attacks.
- **Lost DEK With Live Ciphertext**: When the `crypto.dek` row is missing, `EnableEnvelope` asks the store whether the database still holds ciphertext (`SecretStore.HasEncryptedValues`: encrypted settings keys, notification channel configs, webhook secrets, TOTP shared secrets). A `true` answer returns `ErrDEKMissing` and the boot stops. A fresh DEK in that state would leave every stored value unreadable with no error at the write site. No DEK row is written on that path, so a restart cannot paper over the problem.

## 4. Verification
- Validated via unit tests, rotation tests, and `crypto_characterization_test.go` under `-race`.
