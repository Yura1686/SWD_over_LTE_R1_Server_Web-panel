export class DeviceMap {
  constructor(containerId) {
    this.map = null;
    this.markers = new Map();
    this.coordsNode = null;
    this.coordsRaf = 0;
    this.pendingCoords = null;

    const container = document.getElementById(containerId);
    if (!container) {
      return;
    }

    if (typeof window.L === "undefined") {
      container.innerHTML = '<div class="map-fallback">Map engine is unavailable in this browser/session.</div>';
      return;
    }

    const L = window.L;
    this.map = L.map(containerId, {
      zoomControl: false,
      preferCanvas: true,
      worldCopyJump: true,
      attributionControl: false,
      scrollWheelZoom: true,
      wheelPxPerZoomLevel: 110,
      wheelDebounceTime: 45,
      inertia: true,
      inertiaDeceleration: 2400,
      inertiaMaxSpeed: 1300,
      easeLinearity: 0.2,
      minZoom: 2.5,
      zoomSnap: 0.5,
      zoomDelta: 0.5,
      zoomAnimation: true,
      zoomAnimationThreshold: 6,
      fadeAnimation: false,
      markerZoomAnimation: false,
    }).setView([48.3794, 31.1656], 5);

    L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png", {
      subdomains: "abc",
      maxZoom: 19,
      updateWhenIdle: true,
      updateWhenZooming: false,
      keepBuffer: 2,
      crossOrigin: true,
      detectRetina: false,
    }).addTo(this.map);

    L.control
      .attribution({ position: "bottomright", prefix: false })
      .addAttribution("&copy; OpenStreetMap contributors")
      .addTo(this.map);

    L.control.zoom({ position: "bottomright" }).addTo(this.map);
    L.control.scale({ imperial: false, maxWidth: 120, position: "bottomleft" }).addTo(this.map);

    const coordsControl = L.control({ position: "bottomleft" });
    coordsControl.onAdd = () => {
      this.coordsNode = L.DomUtil.create("div", "map-coords");
      this.coordsNode.textContent = "LAT ---.-----  LON ---.-----  Z --.-";
      return this.coordsNode;
    };
    coordsControl.addTo(this.map);

    const northControl = L.control({ position: "topleft" });
    northControl.onAdd = () => {
      const northNode = L.DomUtil.create("div", "map-north");
      northNode.textContent = "N";
      return northNode;
    };
    northControl.addTo(this.map);

    this.map.on("mousemove", (event) => {
      this.scheduleCoordsUpdate(event.latlng.lat, event.latlng.lng);
    });

    this.map.on("zoomend", () => {
      const center = this.map.getCenter();
      this.scheduleCoordsUpdate(center.lat, center.lng);
      this.refreshMarkerScale();
    });

    this.map.on("moveend", () => {
      const center = this.map.getCenter();
      this.scheduleCoordsUpdate(center.lat, center.lng);
    });

    this.map.on("mouseout", () => {
      const center = this.map.getCenter();
      this.scheduleCoordsUpdate(center.lat, center.lng);
    });

    const center = this.map.getCenter();
    this.scheduleCoordsUpdate(center.lat, center.lng);
  }

  createMarkerIcon(isOnline) {
    // Circle: r=10, canvas=44x44 (extra room for glow/pulse rings)
    // iconAnchor = [22,22] = exact centre of canvas = centre of circle
    const fill  = isOnline ? "#1ec97a"               : "#d94040";
    const neon  = isOnline ? "rgba(50,255,150,0.9)"  : "rgba(255,70,70,0.9)";
    const glow  = isOnline ? "rgba(40,220,130,0.45)" : "rgba(220,60,60,0.4)";
    const pulse = isOnline ? "map-pulse-green"        : "map-pulse-red";

    const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="44" height="44" viewBox="0 0 44 44">
      <defs>
        <radialGradient id="cg${isOnline ? 1 : 0}" cx="40%" cy="32%" r="60%">
          <stop offset="0%"   stop-color="${isOnline ? '#7fffd4' : '#ffb0b0'}" stop-opacity="0.28"/>
          <stop offset="50%"  stop-color="${fill}" stop-opacity="0.82"/>
          <stop offset="100%" stop-color="${isOnline ? '#0a4d30' : '#4d0f0f'}"/>
        </radialGradient>
      </defs>

      <!-- soft glow behind circle -->
      <circle cx="22" cy="22" r="15" fill="${glow}"/>

      <!-- main filled circle -->
      <circle cx="22" cy="22" r="10" fill="url(#cg${isOnline ? 1 : 0})"/>

      <!-- neon border ring -->
      <circle cx="22" cy="22" r="10" fill="none" stroke="${neon}" stroke-width="1.5" opacity="0.95"/>

      <!-- subtle inner highlight -->
      <circle cx="18" cy="17" r="3" fill="white" opacity="0.07"/>

      <!-- animated pulse ring -->
      <circle cx="22" cy="22" r="12" fill="none" stroke="${neon}" stroke-width="1" opacity="0" class="${pulse}"/>
    </svg>`;

    return window.L.divIcon({
      className:    "",         // no extra Leaflet wrapper styles
      html:         svg,
      iconSize:     [44, 44],   // SVG canvas size
      iconAnchor:   [22, 22],   // exact centre — anchor never drifts on zoom
      tooltipAnchor:[0, -18],
    });
  }

  applyMarkerScale(_marker) { /* no-op */ }
  refreshMarkerScale()       { /* no-op */ }

  resize() {
    if (!this.map) {
      return;
    }
    setTimeout(() => {
      this.map.invalidateSize(true);
    }, 60);
  }

  updateDevices(devices) {
    if (!this.map || typeof window.L === "undefined") {
      return;
    }

    const L = window.L;
    const activeIds = new Set();

    devices.forEach((device) => {
      if (!device.last_location) {
        return;
      }

      const markerId = device.device_id;
      activeIds.add(markerId);

      const lat = Number(device.last_location.lat);
      const lon = Number(device.last_location.lon);
      if (!Number.isFinite(lat) || !Number.isFinite(lon)) {
        return;
      }

      const isOnline = device.status === "online";
      const statusLabel = isOnline ? "online" : "offline";
      const label = `${device.device_id} — ${statusLabel}`;
      const icon = this.createMarkerIcon(isOnline);

      if (!this.markers.has(markerId)) {
        const marker = L.marker([lat, lon], {
          icon,
          keyboard: false,
          riseOnHover: true,
          autoPanOnFocus: false,
        }).addTo(this.map);

        marker.bindTooltip(label, {
          direction: "top",
          offset: [0, -14],
          opacity: 0.9,
          className: "map-tooltip",
        });

        this.markers.set(markerId, { marker, isOnline });
        this.applyMarkerScale(marker);
      } else {
        const entry = this.markers.get(markerId);
        entry.marker.setLatLng([lat, lon]);
        entry.marker.setTooltipContent(label);
        if (entry.isOnline !== isOnline) {
          entry.marker.setIcon(icon);
          entry.isOnline = isOnline;
        }
        this.applyMarkerScale(entry.marker);
      }
    });

    Array.from(this.markers.entries()).forEach(([markerId, marker]) => {
      if (!activeIds.has(markerId)) {
        marker.marker.remove();
        this.markers.delete(markerId);
      }
    });
  }

  focusDevice(device) {
    if (!this.map || !device?.last_location) {
      return;
    }

    const lat = Number(device.last_location.lat);
    const lon = Number(device.last_location.lon);
    if (!Number.isFinite(lat) || !Number.isFinite(lon)) {
      return;
    }

    this.map.flyTo([lat, lon], 17, {
      animate: true,
      duration: 0.8,
      easeLinearity: 0.2,
    });
  }

  updateCoords(lat, lon) {
    if (!this.map || !this.coordsNode) {
      return;
    }

    const zoom = this.map.getZoom().toFixed(1);
    this.coordsNode.textContent = `LAT ${lat.toFixed(5)}  LON ${lon.toFixed(5)}  Z ${zoom}`;
  }

  scheduleCoordsUpdate(lat, lon) {
    if (typeof window === "undefined") {
      this.updateCoords(lat, lon);
      return;
    }

    this.pendingCoords = { lat, lon };
    if (this.coordsRaf) {
      return;
    }

    this.coordsRaf = window.requestAnimationFrame(() => {
      this.coordsRaf = 0;
      if (!this.pendingCoords) {
        return;
      }
      const pending = this.pendingCoords;
      this.pendingCoords = null;
      this.updateCoords(pending.lat, pending.lon);
    });
  }
}