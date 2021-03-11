var net = require('net');
var sass = require('node-sass');
var deasync = require('deasync');

if (!process.env.ASSETGEN_SOCK) {
  console.error('error:', 'ASSETGEN_SOCK is not set');
  process.exit(1);
}

// doReq sends a request to the ipc callback server.
function doReq(params, callback) {
  var cl = net.createConnection(process.env.ASSETGEN_SOCK)
  cl.on('connect', function() {
    // connect
    cl.write(JSON.stringify(params) + '\n');
  });
  cl.on('data', function(data) {
    // data
    var p = JSON.parse(data);
    if (p && p.error) {
      console.error('error:', p.error);
      process.exit(1);
    } else if (p && !p.result) {
      console.error('error: missing result');
      process.exit(1);
    }
    callback(p.result);
    cl.destroy();
  });
  cl.on('error', function(e) {
    cl.destroy();
    console.error('error:', e);
    process.exit(1);
  });
}

// conv handles recursively converting a sass type.
function conv(v) {
  if (v instanceof Array) {
    var l = sass.types.List(v.length);
    for (var i = 0; i < v.length; i++) {
      l.setValue(i, conv(v[i]));
    }
    return l
  } else if (v instanceof Object) {
    var keys = Object.keys(v);
    var m = sass.types.Map(keys.length);
    for (var i = 0; i < keys.length; i++) {
      m.setKey(i, conv(keys[i]));
      m.setValue(i, conv(v[keys[i]]));
    }
    return m;
  }
  var typ = typeof v;
  return sass.types[typ.charAt(0).toUpperCase() + typ.slice(1)](v);
}

// doCall performs a call ipc callback server.
function doCall(fn) {
  return function() {
    var args = [];
    for (var i = 0; i < arguments.length - 1; i++) {
      args[i] = arguments[i].getValue();
    }
    var done = false;
    var ret = null;
    doReq({type : 'call', params : {name : fn, args : args}}, function(r) {
      ret = r;
      done = true;
    });
    deasync.loopWhile(function() { return !done; });
    return conv(ret);
  };
}

// get callback list
var done = false;
doReq({type : 'list-functions'}, function(r) {
  module.exports = {};
  for (var i = 0; i < r.length; i++) {
    var fn = r[i];
    module.exports[fn] = doCall(fn);
  }
  done = true;
});
deasync.loopWhile(function() { return !done; });
