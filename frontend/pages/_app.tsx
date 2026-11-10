import { GeistMono } from 'geist/font/mono';
import { GeistSans } from 'geist/font/sans';
import type { AppProps } from 'next/app';
import Head from 'next/head';
import { useRouter } from 'next/router';
import { useEffect } from 'react';
import ForkyLoader from '../components/ForkyLoader';
import Layout from '../components/Layout';
import { ConfirmProvider } from '../components/Modal';
import UpgradeModal from '../components/UpgradeModal';
import { SessionProvider, useSession } from '../lib/session';
import { ToastProvider } from '../lib/toast';
import '../styles/globals.css';

const publicPaths = ['/login'];

function Shell({ Component, pageProps }: AppProps) {
  const { status } = useSession();
  const router = useRouter();
  const isPublic = publicPaths.includes(router.pathname);

  useEffect(() => {
    if (status === 'signedOut' && !isPublic) router.replace('/login/');
    if (status === 'signedIn' && isPublic) router.replace('/');
  }, [status, isPublic, router]);

  if (status === 'loading') return <ForkyLoader fullScreen />;
  if (isPublic) return status === 'signedOut' ? <Component {...pageProps} /> : null;
  if (status !== 'signedIn') return null;
  return (
    <Layout>
      <Component {...pageProps} />
      <UpgradeModal />
    </Layout>
  );
}

export default function App(props: AppProps) {
  return (
    <div className={`${GeistSans.variable} ${GeistMono.variable} font-sans`}>
      <Head>
        <title>Forke Mail</title>
        <meta name="viewport" content="width=device-width, initial-scale=1" />
      </Head>
      <ToastProvider>
        <SessionProvider>
          <ConfirmProvider>
            <Shell {...props} />
          </ConfirmProvider>
        </SessionProvider>
      </ToastProvider>
    </div>
  );
}
