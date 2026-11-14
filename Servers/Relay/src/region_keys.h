// Region signing key: generation and loading (PROTOCOL.md sections 4.3 and 7.2, key_file).
#pragma once

#include <cstddef>
#include <string>

namespace relay {

const std::size_t REGION_SECRET_KEY_SIZE = 64; // crypto_sign_SECRETKEYBYTES
const std::size_t REGION_PUBLIC_KEY_SIZE = 32; // crypto_sign_PUBLICKEYBYTES

struct RegionKey {
  unsigned char secret[REGION_SECRET_KEY_SIZE] = {};
  unsigned char public_key[REGION_PUBLIC_KEY_SIZE] = {};
};

// Creates a new region key pair. The secret key is written as one line of hex to secret_path, and the public key
// as one line of hex to secret_path + ".pub". Existing files are never overwritten. On failure, error says why.
bool GenerateRegionKeys(const std::string &secret_path, std::string &error);

// Reads the secret key file written by GenerateRegionKeys. The public key is derived from the seed in the file,
// and the stored key must match it. On failure, error says why.
bool LoadRegionKey(const std::string &secret_path, RegionKey &key, std::string &error);

} // namespace relay
