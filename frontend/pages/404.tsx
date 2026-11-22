/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import { Compass } from 'lucide-react';
import { Card, Empty, ButtonLink } from '../components/ui';

export default function NotFound() {
  return (
    <Card>
      <Empty icon={Compass} title="Page not found" hint="The page you are looking for does not exist." action={<ButtonLink href="/">Back to overview</ButtonLink>} />
    </Card>
  );
}
