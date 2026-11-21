// Adapted from the shadcn/ui Sidebar (MIT, see SHADCN-LICENSE.md).
// The admin shell attaches navigation data and controls the desktop open state.
import { Slot } from "@radix-ui/react-slot";

export function SidebarHeader() {
  return <div data-slot="sidebar-header" data-sidebar="header" className="flex flex-col gap-2 p-2" />;
}

export function SidebarFooter() {
  return <div data-slot="sidebar-footer" data-sidebar="footer" className="flex flex-col gap-2 p-2" />;
}

export function SidebarContent() {
  return <nav data-slot="sidebar-content" data-sidebar="content" className="flex min-h-0 flex-1 flex-col gap-2 overflow-auto" />;
}

export function SidebarGroup() {
  return <div data-slot="sidebar-group" data-sidebar="group" className="relative flex w-full min-w-0 flex-col p-2" />;
}

export function SidebarGroupLabel() {
  return <div data-slot="sidebar-group-label" data-sidebar="group-label" className="flex h-8 shrink-0 items-center rounded-md px-2 text-xs font-medium" />;
}

export function SidebarMenu() {
  return <ul data-slot="sidebar-menu" data-sidebar="menu" className="flex w-full min-w-0 flex-col gap-1" />;
}

export function SidebarMenuItem() {
  return <li data-slot="sidebar-menu-item" data-sidebar="menu-item" className="group/menu-item relative" />;
}

export function SidebarMenuButton() {
  return (
    <Slot data-slot="sidebar-menu-button" data-sidebar="menu-button" className="peer/menu-button flex w-full items-center gap-2 overflow-hidden rounded-md p-2 text-left text-sm data-[active=true]:font-medium [&>span:last-child]:truncate">
      <a />
    </Slot>
  );
}
