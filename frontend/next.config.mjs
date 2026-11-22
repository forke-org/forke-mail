/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

const dev = process.env.NODE_ENV === 'development';

const nextConfig = {
  output: dev ? undefined : 'export',
  trailingSlash: true,
  skipTrailingSlashRedirect: dev,
  reactStrictMode: true,
  poweredByHeader: false,
  agentRules: false,
  experimental: { agentFeedback: false },
  images: { unoptimized: true },
  rewrites: dev
    ? async () => [{ source: '/api/:path*', destination: 'http://localhost:8080/api/:path*' }]
    : undefined,
};

export default nextConfig;
