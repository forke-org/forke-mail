import { GeistMono } from 'geist/font/mono';
import { GeistSans } from 'geist/font/sans';
import type { AppProps } from 'next/app';
import Head from 'next/head';
import { useRouter } from 'next/router';
import { useEffect, useState, type ReactNode } from 'react';
import ForkyLoader, { useLoaderMinimum } from '../components/ForkyLoader';
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

  const minimum = useLoaderMinimum();
  const [gate, setGate] = useState(true);
  const redirecting = (status === 'signedOut' && !isPublic) || (status === 'signedIn' && isPublic);
  const ready = status !== 'loading' && !redirecting;

  // What the page shows once the session is known. Kept separate from the loader so the
  // loader always sits at the same spot in the tree: if it moved between branches, React
  // would remount it mid-drawing and the animation would jump back to the start.
  let content: ReactNode = null;
  if (ready && isPublic) content = <Component {...pageProps} />;
  if (ready && !isPublic)
    content = (
      <Layout>
        <Component {...pageProps} />
        <UpgradeModal />
      </Layout>
    );

  return (
    <>
      {content}
      {gate ? <ForkyLoader fullScreen exiting={ready && minimum} onExited={() => setGate(false)} /> : null}
    </>
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
