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
