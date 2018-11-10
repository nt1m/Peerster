function $(sel, target = document) {
  return target.querySelector(sel);
}

function $$(sel, target = document) {
  return target.querySelectorAll(sel);
}

function setupTabComponent() {
  for (const element of $$(".tabs")) {
    const nav = $(".tab-nav", element);
    for (const tab of $$(".tab", nav)) {
      tab.addEventListener("click", function (event) {
        var newSelected = "[data-tab=" + tab.dataset.tab + "]";
        var currentSelected = $$(".tab-panel.selected, .tab.selected", element);
        if (currentSelected.length > 0) {
          for (const panel of currentSelected) {
            panel.classList.remove("selected");
          }
        }
        for (const selected of $$(".tab" + newSelected + ", .tab-panel" + newSelected, element)) {
          selected.classList.add("selected");
        }
        event.preventDefault();
      });
    }
    // Select first element
    nav.firstElementChild.click();
  }
}

function hashCode(str) { // java String#hashCode
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
     hash = str.charCodeAt(i) + ((hash << 5) - hash);
  }
  return hash;
}

function hashColor(str){
  const i = hashCode(str);
  const c = (i & 0x00FFFFFF)
      .toString(16)
      .toUpperCase();
  const background = "00000".substring(0, 6 - c.length) + c;
  const r = parseInt(background.substring(0, 2), 16);
  const g = parseInt(background.substring(2, 4), 16);
  const b = parseInt(background.substring(4, 6), 16);

  const isBackgroundDark = (0.2125 * r + 0.7154 * g + 0.0721 * b) <= 110;
  const textColor = isBackgroundDark ? "#ffffff" : "#000000";
  return {background: "#" + background, textColor};
}

document.addEventListener("DOMContentLoaded", async () => {
  $("#node-id").textContent = await getNodeId();

  updateStatus();
  setInterval(updateStatus, 3000);

  $("#message-form").addEventListener("submit", e => {
    e.preventDefault();
    sendMessage($("#send-input").value, $("#send-destination").value);
    $("#send-input").value = "";
  });

  $("#file-upload-form").addEventListener("submit", e => {
    e.preventDefault();
    sendUploadRequest($("#file-upload-path").value);
    $("#file-upload-path").value = "";
  });

  $("#file-request-form").addEventListener("submit", e => {
    e.preventDefault();
    sendDownloadRequest(
      $("#file-requestee").value,
      $("#file-request-name").value,
      $("#file-request-hash").value,
    );
    $("#file-request-name").value = "";
    $("#file-request-hash").value = "";
  });

  $("#add-peer-form").addEventListener("submit", e => {
    e.preventDefault();
    addPeer($("#add-peer-input").value);
    $("#add-peer-input").value = "";
  });
  setupTabComponent();
});

let receivedMessages = 0;

async function updateStatus() {
  const peers = await getAllPeers();
  $("#node-peers").textContent = "";
  $("#node-peers").append(...peers.map((p) => {
    const li = document.createElement("li");
    li.textContent = p;
    return li;
  }));

  const files = await getAllFiles();
  $("#node-files").textContent = "";
  $("#node-files").append(...files.map(({Name, Hash}) => {
    const li = document.createElement("li");

    const name = document.createElement("span");
    name.className = "file-name";
    name.textContent = Name;
    const hash = document.createElement("span");
    hash.className = "file-hash";
    hash.textContent = Hash;

    li.append(name, hash);
    return li;
  }));

  const messages = await getAllMessages();
  $("#messages").append(...messages.slice(receivedMessages).map(({ Origin, ID, Text, Destination }) => {
    const li = document.createElement("li");
    li.classList.toggle("private", !!Destination);

    const originEl = document.createElement("span");
    originEl.textContent = Origin;
    originEl.className = "message-origin";

    const {background, textColor} = hashColor(Origin);
    originEl.style.backgroundColor = background;
    originEl.style.color = textColor;

    const idEl = document.createElement("span");
    if (!Destination) {
      idEl.textContent = "Message " + ID;
    } else if ($("#node-id").textContent != Destination) {
      idEl.textContent = "Private message to " + Destination;
    } else {
      idEl.textContent = "Private message";
    }
    idEl.className = "message-id";

    const contentsEl = document.createElement("p");
    contentsEl.textContent = Text;
    contentsEl.className = "message-contents";

    li.append(originEl, idEl, contentsEl);
    return li;
  }));
  receivedMessages = messages.length;

  await updateDestinationSelect($("#send-destination"), true)
  await updateDestinationSelect($("#file-requestee"), false)
}

async function updateDestinationSelect(select, includeEveryone) {
  let destinations = await getAllDestinations();
  if (includeEveryone) {
    destinations = ["", ...destinations];
  }
  const lastDestination = select.value;
  select.textContent = "";
  select.append(...destinations.sort((a,b) => a == "" ? -1 : a > b).map(d => {
    const option = document.createElement("option");
    option.value = d;
    option.textContent = d == "" ? "Everyone" : d;
    return option;
  }));
  select.value = destinations.includes(lastDestination) ? lastDestination : select.firstElementChild.value;
}

async function getNodeId() {
  const response = await fetch("/id");
  return await response.text();
}

async function getAllPeers() {
  const response = await fetch("/node");
  return JSON.parse(await response.text());
}

async function getAllFiles() {
  const response = await fetch("/file");
  return JSON.parse(await response.text()).sort((a, b) => a.Name > b.Name);
}

async function getAllMessages() {
  const response = await fetch("/message");
  return JSON.parse(await response.text());
}

async function getAllDestinations() {
  const response = await fetch("/destination");
  return [...JSON.parse(await response.text())];
}

function sendMessage(message, destination) {
  const headers = new Headers();
  headers.append("Content-Type", "application/json");

  return fetch("/message", {
    method: "POST",
    headers,
    body: JSON.stringify({
      Text: message,
      Destination: destination,
    }),
  });
}

function sendUploadRequest(path) {
  const headers = new Headers();
  headers.append("Content-Type", "application/json");

  return fetch("/message", {
    method: "POST",
    headers,
    body: JSON.stringify({
      File: path,
    }),
  });
}

function sendDownloadRequest(destination, path, hash) {
  const headers = new Headers();
  headers.append("Content-Type", "application/json");

  return fetch("/message", {
    method: "POST",
    headers,
    body: JSON.stringify({
      File: path,
      Destination: destination,
      Request: hash
    }),
  });
}

function addPeer(peerAddr) {
  const headers = new Headers();
  headers.append("Content-Type", "text/plain");

  return fetch("/node", {
    method: "POST",
    headers,
    body: peerAddr,
  });
}
