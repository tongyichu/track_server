import { renderToStaticMarkup } from "react-dom/server";
import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";

const components = {
  alert: Alert, badge: Badge, button: Button, card: Card, "card-content": CardContent, "card-header": CardHeader,
  input: Input, table: Table, tbody: TableBody, td: TableCell, th: TableHead, thead: TableHeader, tr: TableRow, textarea: Textarea,
};

export function componentMarkup(name, props) {
  const Component = components[name];
  return renderToStaticMarkup(<Component {...props} />);
}
