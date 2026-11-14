#include "region_keys.h"

#include "text.h"

#include <cstdio>
#include <fstream>
#include <sodium.h>
#include <sstream>

namespace relay {

namespace {

const std::size_t MAX_KEY_FILE_BYTES = 1024;
const char *const PUBLIC_KEY_SUFFIX = ".pub";

// Writes text as a new file. Returns false if the file already exists or cannot be written.
bool WriteNewFile(const std::string &path, const std::string &text, std::string &error) {
  std::FILE *file = std::fopen(path.c_str(), "wx");
  if (file == nullptr) {
    error = "cannot create '" + path + "' (it may already exist)";
    return false;
  }
  bool written = std::fputs(text.c_str(), file) >= 0 && std::fputc('\n', file) != EOF;
  bool closed = std::fclose(file) == 0;
  if (!written || !closed) {
    error = "cannot write '" + path + "'";
    static_cast<void>(std::remove(path.c_str()));
    return false;
  }
  return true;
}

std::string ToHex(const unsigned char *bytes, std::size_t length) {
  std::string hex((length * 2) + 1, '\0');
  sodium_bin2hex(hex.data(), hex.size(), bytes, length);
  hex.resize(length * 2);
  return hex;
}

} // namespace

bool GenerateRegionKeys(const std::string &secret_path, std::string &error) {
  RegionKey key;
  if (crypto_sign_keypair(key.public_key, key.secret) != 0) {
    error = "key generation failed";
    return false;
  }

  std::string public_path = secret_path + PUBLIC_KEY_SUFFIX;
  std::FILE *probe = std::fopen(public_path.c_str(), "rb");
  if (probe != nullptr) {
    static_cast<void>(std::fclose(probe));
    error = "'" + public_path + "' already exists";
    return false;
  }

  if (!WriteNewFile(secret_path, ToHex(key.secret, sizeof(key.secret)), error)) {
    sodium_memzero(key.secret, sizeof(key.secret));
    return false;
  }
  if (!WriteNewFile(public_path, ToHex(key.public_key, sizeof(key.public_key)), error)) {
    static_cast<void>(std::remove(secret_path.c_str()));
    sodium_memzero(key.secret, sizeof(key.secret));
    return false;
  }

  sodium_memzero(key.secret, sizeof(key.secret));
  return true;
}

bool LoadRegionKey(const std::string &secret_path, RegionKey &key, std::string &error) {
  std::ifstream file(secret_path.c_str(), std::ios::in | std::ios::binary);
  if (!file) {
    error = "cannot open key file '" + secret_path + "'";
    return false;
  }

  std::stringstream buffer;
  buffer << file.rdbuf();
  std::string text = buffer.str();
  if (text.size() > MAX_KEY_FILE_BYTES) {
    error = "key file '" + secret_path + "' is too large";
    return false;
  }

  std::string hex = TrimText(text);
  std::size_t decoded_length = 0;
  RegionKey loaded;
  if (sodium_hex2bin(loaded.secret, sizeof(loaded.secret), hex.c_str(), hex.size(), nullptr, &decoded_length,
                     nullptr) != 0 ||
      decoded_length != REGION_SECRET_KEY_SIZE) {
    error = "key file '" + secret_path + "' does not hold a valid secret key";
    sodium_memzero(loaded.secret, sizeof(loaded.secret));
    return false;
  }

  // The first 32 bytes of the secret key are the seed. Derive the pair again and require it to match.
  RegionKey derived;
  if (crypto_sign_seed_keypair(derived.public_key, derived.secret, loaded.secret) != 0 ||
      sodium_memcmp(derived.secret, loaded.secret, REGION_SECRET_KEY_SIZE) != 0) {
    error = "key file '" + secret_path + "' is not a consistent key pair";
    sodium_memzero(loaded.secret, sizeof(loaded.secret));
    sodium_memzero(derived.secret, sizeof(derived.secret));
    return false;
  }

  key = derived;
  sodium_memzero(loaded.secret, sizeof(loaded.secret));
  sodium_memzero(derived.secret, sizeof(derived.secret));
  return true;
}

} // namespace relay
