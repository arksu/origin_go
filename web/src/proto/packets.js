/*eslint-disable block-scoped-var, id-length, no-control-regex, no-magic-numbers, no-prototype-builtins, no-redeclare, no-shadow, no-var, sort-vars*/
import * as $protobuf from "protobufjs/minimal";

// Common aliases
const $Reader = $protobuf.Reader, $Writer = $protobuf.Writer, $util = $protobuf.util;

// Exported root namespace
const $root = $protobuf.roots["default"] || ($protobuf.roots["default"] = {});

export const proto = $root.proto = (() => {

    /**
     * Namespace proto.
     * @exports proto
     * @namespace
     */
    const proto = {};

    proto.Position = (function() {

        /**
         * Properties of a Position.
         * @memberof proto
         * @interface IPosition
         * @property {number|null} [x] Position x
         * @property {number|null} [y] Position y
         * @property {number|null} [heading] Position heading
         */

        /**
         * Constructs a new Position.
         * @memberof proto
         * @classdesc Represents a Position.
         * @implements IPosition
         * @constructor
         * @param {proto.IPosition=} [properties] Properties to set
         */
        function Position(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Position x.
         * @member {number} x
         * @memberof proto.Position
         * @instance
         */
        Position.prototype.x = 0;

        /**
         * Position y.
         * @member {number} y
         * @memberof proto.Position
         * @instance
         */
        Position.prototype.y = 0;

        /**
         * Position heading.
         * @member {number} heading
         * @memberof proto.Position
         * @instance
         */
        Position.prototype.heading = 0;

        /**
         * Creates a new Position instance using the specified properties.
         * @function create
         * @memberof proto.Position
         * @static
         * @param {proto.IPosition=} [properties] Properties to set
         * @returns {proto.Position} Position instance
         */
        Position.create = function create(properties) {
            return new Position(properties);
        };

        /**
         * Encodes the specified Position message. Does not implicitly {@link proto.Position.verify|verify} messages.
         * @function encode
         * @memberof proto.Position
         * @static
         * @param {proto.IPosition} message Position message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Position.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.x != null && Object.hasOwnProperty.call(message, "x"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.x);
            if (message.y != null && Object.hasOwnProperty.call(message, "y"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.y);
            if (message.heading != null && Object.hasOwnProperty.call(message, "heading"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.heading);
            return writer;
        };

        /**
         * Encodes the specified Position message, length delimited. Does not implicitly {@link proto.Position.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.Position
         * @static
         * @param {proto.IPosition} message Position message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Position.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Position message from the specified reader or buffer.
         * @function decode
         * @memberof proto.Position
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.Position} Position
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Position.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.Position();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.x = reader.int32();
                        break;
                    }
                case 2: {
                        message.y = reader.int32();
                        break;
                    }
                case 3: {
                        message.heading = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a Position message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.Position
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.Position} Position
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Position.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Position message.
         * @function verify
         * @memberof proto.Position
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Position.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.x != null && message.hasOwnProperty("x"))
                if (!$util.isInteger(message.x))
                    return "x: integer expected";
            if (message.y != null && message.hasOwnProperty("y"))
                if (!$util.isInteger(message.y))
                    return "y: integer expected";
            if (message.heading != null && message.hasOwnProperty("heading"))
                if (!$util.isInteger(message.heading))
                    return "heading: integer expected";
            return null;
        };

        /**
         * Creates a Position message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.Position
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.Position} Position
         */
        Position.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.Position)
                return object;
            let message = new $root.proto.Position();
            if (object.x != null)
                message.x = object.x | 0;
            if (object.y != null)
                message.y = object.y | 0;
            if (object.heading != null)
                message.heading = object.heading >>> 0;
            return message;
        };

        /**
         * Creates a plain object from a Position message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.Position
         * @static
         * @param {proto.Position} message Position
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Position.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.x = 0;
                object.y = 0;
                object.heading = 0;
            }
            if (message.x != null && message.hasOwnProperty("x"))
                object.x = message.x;
            if (message.y != null && message.hasOwnProperty("y"))
                object.y = message.y;
            if (message.heading != null && message.hasOwnProperty("heading"))
                object.heading = message.heading;
            return object;
        };

        /**
         * Converts this Position to JSON.
         * @function toJSON
         * @memberof proto.Position
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Position.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Position
         * @function getTypeUrl
         * @memberof proto.Position
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Position.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.Position";
        };

        return Position;
    })();

    proto.Vector2 = (function() {

        /**
         * Properties of a Vector2.
         * @memberof proto
         * @interface IVector2
         * @property {number|null} [x] Vector2 x
         * @property {number|null} [y] Vector2 y
         */

        /**
         * Constructs a new Vector2.
         * @memberof proto
         * @classdesc Represents a Vector2.
         * @implements IVector2
         * @constructor
         * @param {proto.IVector2=} [properties] Properties to set
         */
        function Vector2(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Vector2 x.
         * @member {number} x
         * @memberof proto.Vector2
         * @instance
         */
        Vector2.prototype.x = 0;

        /**
         * Vector2 y.
         * @member {number} y
         * @memberof proto.Vector2
         * @instance
         */
        Vector2.prototype.y = 0;

        /**
         * Creates a new Vector2 instance using the specified properties.
         * @function create
         * @memberof proto.Vector2
         * @static
         * @param {proto.IVector2=} [properties] Properties to set
         * @returns {proto.Vector2} Vector2 instance
         */
        Vector2.create = function create(properties) {
            return new Vector2(properties);
        };

        /**
         * Encodes the specified Vector2 message. Does not implicitly {@link proto.Vector2.verify|verify} messages.
         * @function encode
         * @memberof proto.Vector2
         * @static
         * @param {proto.IVector2} message Vector2 message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Vector2.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.x != null && Object.hasOwnProperty.call(message, "x"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.x);
            if (message.y != null && Object.hasOwnProperty.call(message, "y"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.y);
            return writer;
        };

        /**
         * Encodes the specified Vector2 message, length delimited. Does not implicitly {@link proto.Vector2.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.Vector2
         * @static
         * @param {proto.IVector2} message Vector2 message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Vector2.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Vector2 message from the specified reader or buffer.
         * @function decode
         * @memberof proto.Vector2
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.Vector2} Vector2
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Vector2.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.Vector2();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.x = reader.int32();
                        break;
                    }
                case 2: {
                        message.y = reader.int32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a Vector2 message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.Vector2
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.Vector2} Vector2
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Vector2.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Vector2 message.
         * @function verify
         * @memberof proto.Vector2
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Vector2.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.x != null && message.hasOwnProperty("x"))
                if (!$util.isInteger(message.x))
                    return "x: integer expected";
            if (message.y != null && message.hasOwnProperty("y"))
                if (!$util.isInteger(message.y))
                    return "y: integer expected";
            return null;
        };

        /**
         * Creates a Vector2 message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.Vector2
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.Vector2} Vector2
         */
        Vector2.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.Vector2)
                return object;
            let message = new $root.proto.Vector2();
            if (object.x != null)
                message.x = object.x | 0;
            if (object.y != null)
                message.y = object.y | 0;
            return message;
        };

        /**
         * Creates a plain object from a Vector2 message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.Vector2
         * @static
         * @param {proto.Vector2} message Vector2
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Vector2.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.x = 0;
                object.y = 0;
            }
            if (message.x != null && message.hasOwnProperty("x"))
                object.x = message.x;
            if (message.y != null && message.hasOwnProperty("y"))
                object.y = message.y;
            return object;
        };

        /**
         * Converts this Vector2 to JSON.
         * @function toJSON
         * @memberof proto.Vector2
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Vector2.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Vector2
         * @function getTypeUrl
         * @memberof proto.Vector2
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Vector2.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.Vector2";
        };

        return Vector2;
    })();

    proto.AABB = (function() {

        /**
         * Properties of a AABB.
         * @memberof proto
         * @interface IAABB
         * @property {number|null} [minX] AABB minX
         * @property {number|null} [minY] AABB minY
         * @property {number|null} [maxX] AABB maxX
         * @property {number|null} [maxY] AABB maxY
         */

        /**
         * Constructs a new AABB.
         * @memberof proto
         * @classdesc Represents a AABB.
         * @implements IAABB
         * @constructor
         * @param {proto.IAABB=} [properties] Properties to set
         */
        function AABB(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * AABB minX.
         * @member {number} minX
         * @memberof proto.AABB
         * @instance
         */
        AABB.prototype.minX = 0;

        /**
         * AABB minY.
         * @member {number} minY
         * @memberof proto.AABB
         * @instance
         */
        AABB.prototype.minY = 0;

        /**
         * AABB maxX.
         * @member {number} maxX
         * @memberof proto.AABB
         * @instance
         */
        AABB.prototype.maxX = 0;

        /**
         * AABB maxY.
         * @member {number} maxY
         * @memberof proto.AABB
         * @instance
         */
        AABB.prototype.maxY = 0;

        /**
         * Creates a new AABB instance using the specified properties.
         * @function create
         * @memberof proto.AABB
         * @static
         * @param {proto.IAABB=} [properties] Properties to set
         * @returns {proto.AABB} AABB instance
         */
        AABB.create = function create(properties) {
            return new AABB(properties);
        };

        /**
         * Encodes the specified AABB message. Does not implicitly {@link proto.AABB.verify|verify} messages.
         * @function encode
         * @memberof proto.AABB
         * @static
         * @param {proto.IAABB} message AABB message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        AABB.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.minX != null && Object.hasOwnProperty.call(message, "minX"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.minX);
            if (message.minY != null && Object.hasOwnProperty.call(message, "minY"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.minY);
            if (message.maxX != null && Object.hasOwnProperty.call(message, "maxX"))
                writer.uint32(/* id 3, wireType 0 =*/24).int32(message.maxX);
            if (message.maxY != null && Object.hasOwnProperty.call(message, "maxY"))
                writer.uint32(/* id 4, wireType 0 =*/32).int32(message.maxY);
            return writer;
        };

        /**
         * Encodes the specified AABB message, length delimited. Does not implicitly {@link proto.AABB.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.AABB
         * @static
         * @param {proto.IAABB} message AABB message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        AABB.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a AABB message from the specified reader or buffer.
         * @function decode
         * @memberof proto.AABB
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.AABB} AABB
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        AABB.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.AABB();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.minX = reader.int32();
                        break;
                    }
                case 2: {
                        message.minY = reader.int32();
                        break;
                    }
                case 3: {
                        message.maxX = reader.int32();
                        break;
                    }
                case 4: {
                        message.maxY = reader.int32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a AABB message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.AABB
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.AABB} AABB
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        AABB.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a AABB message.
         * @function verify
         * @memberof proto.AABB
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        AABB.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.minX != null && message.hasOwnProperty("minX"))
                if (!$util.isInteger(message.minX))
                    return "minX: integer expected";
            if (message.minY != null && message.hasOwnProperty("minY"))
                if (!$util.isInteger(message.minY))
                    return "minY: integer expected";
            if (message.maxX != null && message.hasOwnProperty("maxX"))
                if (!$util.isInteger(message.maxX))
                    return "maxX: integer expected";
            if (message.maxY != null && message.hasOwnProperty("maxY"))
                if (!$util.isInteger(message.maxY))
                    return "maxY: integer expected";
            return null;
        };

        /**
         * Creates a AABB message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.AABB
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.AABB} AABB
         */
        AABB.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.AABB)
                return object;
            let message = new $root.proto.AABB();
            if (object.minX != null)
                message.minX = object.minX | 0;
            if (object.minY != null)
                message.minY = object.minY | 0;
            if (object.maxX != null)
                message.maxX = object.maxX | 0;
            if (object.maxY != null)
                message.maxY = object.maxY | 0;
            return message;
        };

        /**
         * Creates a plain object from a AABB message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.AABB
         * @static
         * @param {proto.AABB} message AABB
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        AABB.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.minX = 0;
                object.minY = 0;
                object.maxX = 0;
                object.maxY = 0;
            }
            if (message.minX != null && message.hasOwnProperty("minX"))
                object.minX = message.minX;
            if (message.minY != null && message.hasOwnProperty("minY"))
                object.minY = message.minY;
            if (message.maxX != null && message.hasOwnProperty("maxX"))
                object.maxX = message.maxX;
            if (message.maxY != null && message.hasOwnProperty("maxY"))
                object.maxY = message.maxY;
            return object;
        };

        /**
         * Converts this AABB to JSON.
         * @function toJSON
         * @memberof proto.AABB
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        AABB.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for AABB
         * @function getTypeUrl
         * @memberof proto.AABB
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        AABB.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.AABB";
        };

        return AABB;
    })();

    proto.Timestamp = (function() {

        /**
         * Properties of a Timestamp.
         * @memberof proto
         * @interface ITimestamp
         * @property {number|Long|null} [millis] Timestamp millis
         */

        /**
         * Constructs a new Timestamp.
         * @memberof proto
         * @classdesc Represents a Timestamp.
         * @implements ITimestamp
         * @constructor
         * @param {proto.ITimestamp=} [properties] Properties to set
         */
        function Timestamp(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Timestamp millis.
         * @member {number|Long} millis
         * @memberof proto.Timestamp
         * @instance
         */
        Timestamp.prototype.millis = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

        /**
         * Creates a new Timestamp instance using the specified properties.
         * @function create
         * @memberof proto.Timestamp
         * @static
         * @param {proto.ITimestamp=} [properties] Properties to set
         * @returns {proto.Timestamp} Timestamp instance
         */
        Timestamp.create = function create(properties) {
            return new Timestamp(properties);
        };

        /**
         * Encodes the specified Timestamp message. Does not implicitly {@link proto.Timestamp.verify|verify} messages.
         * @function encode
         * @memberof proto.Timestamp
         * @static
         * @param {proto.ITimestamp} message Timestamp message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Timestamp.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.millis != null && Object.hasOwnProperty.call(message, "millis"))
                writer.uint32(/* id 1, wireType 0 =*/8).int64(message.millis);
            return writer;
        };

        /**
         * Encodes the specified Timestamp message, length delimited. Does not implicitly {@link proto.Timestamp.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.Timestamp
         * @static
         * @param {proto.ITimestamp} message Timestamp message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Timestamp.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Timestamp message from the specified reader or buffer.
         * @function decode
         * @memberof proto.Timestamp
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.Timestamp} Timestamp
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Timestamp.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.Timestamp();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.millis = reader.int64();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a Timestamp message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.Timestamp
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.Timestamp} Timestamp
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Timestamp.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Timestamp message.
         * @function verify
         * @memberof proto.Timestamp
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Timestamp.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.millis != null && message.hasOwnProperty("millis"))
                if (!$util.isInteger(message.millis) && !(message.millis && $util.isInteger(message.millis.low) && $util.isInteger(message.millis.high)))
                    return "millis: integer|Long expected";
            return null;
        };

        /**
         * Creates a Timestamp message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.Timestamp
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.Timestamp} Timestamp
         */
        Timestamp.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.Timestamp)
                return object;
            let message = new $root.proto.Timestamp();
            if (object.millis != null)
                if ($util.Long)
                    (message.millis = $util.Long.fromValue(object.millis)).unsigned = false;
                else if (typeof object.millis === "string")
                    message.millis = parseInt(object.millis, 10);
                else if (typeof object.millis === "number")
                    message.millis = object.millis;
                else if (typeof object.millis === "object")
                    message.millis = new $util.LongBits(object.millis.low >>> 0, object.millis.high >>> 0).toNumber();
            return message;
        };

        /**
         * Creates a plain object from a Timestamp message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.Timestamp
         * @static
         * @param {proto.Timestamp} message Timestamp
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Timestamp.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                if ($util.Long) {
                    let long = new $util.Long(0, 0, false);
                    object.millis = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.millis = options.longs === String ? "0" : 0;
            if (message.millis != null && message.hasOwnProperty("millis"))
                if (typeof message.millis === "number")
                    object.millis = options.longs === String ? String(message.millis) : message.millis;
                else
                    object.millis = options.longs === String ? $util.Long.prototype.toString.call(message.millis) : options.longs === Number ? new $util.LongBits(message.millis.low >>> 0, message.millis.high >>> 0).toNumber() : message.millis;
            return object;
        };

        /**
         * Converts this Timestamp to JSON.
         * @function toJSON
         * @memberof proto.Timestamp
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Timestamp.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Timestamp
         * @function getTypeUrl
         * @memberof proto.Timestamp
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Timestamp.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.Timestamp";
        };

        return Timestamp;
    })();

    /**
     * MoveMode enum.
     * @name proto.MoveMode
     * @enum {number}
     * @property {number} MOVE_MODE_WALK=0 MOVE_MODE_WALK value
     * @property {number} MOVE_MODE_RUN=1 MOVE_MODE_RUN value
     * @property {number} MOVE_MODE_FAST_RUN=2 MOVE_MODE_FAST_RUN value
     * @property {number} MOVE_MODE_SWIM=3 MOVE_MODE_SWIM value
     */
    proto.MoveMode = (function() {
        const valuesById = {}, values = Object.create(valuesById);
        values[valuesById[0] = "MOVE_MODE_WALK"] = 0;
        values[valuesById[1] = "MOVE_MODE_RUN"] = 1;
        values[valuesById[2] = "MOVE_MODE_FAST_RUN"] = 2;
        values[valuesById[3] = "MOVE_MODE_SWIM"] = 3;
        return values;
    })();

    /**
     * EquipSlot enum.
     * @name proto.EquipSlot
     * @enum {number}
     * @property {number} EQUIP_SLOT_NONE=0 EQUIP_SLOT_NONE value
     * @property {number} EQUIP_SLOT_HEAD=1 EQUIP_SLOT_HEAD value
     * @property {number} EQUIP_SLOT_CHEST=2 EQUIP_SLOT_CHEST value
     * @property {number} EQUIP_SLOT_LEGS=3 EQUIP_SLOT_LEGS value
     * @property {number} EQUIP_SLOT_FEET=4 EQUIP_SLOT_FEET value
     * @property {number} EQUIP_SLOT_HANDS=5 EQUIP_SLOT_HANDS value
     * @property {number} EQUIP_SLOT_LEFT_HAND=6 EQUIP_SLOT_LEFT_HAND value
     * @property {number} EQUIP_SLOT_RIGHT_HAND=7 EQUIP_SLOT_RIGHT_HAND value
     * @property {number} EQUIP_SLOT_BACK=8 EQUIP_SLOT_BACK value
     * @property {number} EQUIP_SLOT_NECK=9 EQUIP_SLOT_NECK value
     * @property {number} EQUIP_SLOT_RING_1=10 EQUIP_SLOT_RING_1 value
     * @property {number} EQUIP_SLOT_RING_2=11 EQUIP_SLOT_RING_2 value
     */
    proto.EquipSlot = (function() {
        const valuesById = {}, values = Object.create(valuesById);
        values[valuesById[0] = "EQUIP_SLOT_NONE"] = 0;
        values[valuesById[1] = "EQUIP_SLOT_HEAD"] = 1;
        values[valuesById[2] = "EQUIP_SLOT_CHEST"] = 2;
        values[valuesById[3] = "EQUIP_SLOT_LEGS"] = 3;
        values[valuesById[4] = "EQUIP_SLOT_FEET"] = 4;
        values[valuesById[5] = "EQUIP_SLOT_HANDS"] = 5;
        values[valuesById[6] = "EQUIP_SLOT_LEFT_HAND"] = 6;
        values[valuesById[7] = "EQUIP_SLOT_RIGHT_HAND"] = 7;
        values[valuesById[8] = "EQUIP_SLOT_BACK"] = 8;
        values[valuesById[9] = "EQUIP_SLOT_NECK"] = 9;
        values[valuesById[10] = "EQUIP_SLOT_RING_1"] = 10;
        values[valuesById[11] = "EQUIP_SLOT_RING_2"] = 11;
        return values;
    })();

    /**
     * ExpType enum.
     * @name proto.ExpType
     * @enum {number}
     * @property {number} EXP_TYPE_NATURE=0 EXP_TYPE_NATURE value
     * @property {number} EXP_TYPE_INDUSTRY=1 EXP_TYPE_INDUSTRY value
     * @property {number} EXP_TYPE_COMBAT=2 EXP_TYPE_COMBAT value
     */
    proto.ExpType = (function() {
        const valuesById = {}, values = Object.create(valuesById);
        values[valuesById[0] = "EXP_TYPE_NATURE"] = 0;
        values[valuesById[1] = "EXP_TYPE_INDUSTRY"] = 1;
        values[valuesById[2] = "EXP_TYPE_COMBAT"] = 2;
        return values;
    })();

    /**
     * WeatherType enum.
     * @name proto.WeatherType
     * @enum {number}
     * @property {number} WEATHER_TYPE_CLEAR=0 WEATHER_TYPE_CLEAR value
     * @property {number} WEATHER_TYPE_RAIN=1 WEATHER_TYPE_RAIN value
     * @property {number} WEATHER_TYPE_FOG=2 WEATHER_TYPE_FOG value
     * @property {number} WEATHER_TYPE_STORM=3 WEATHER_TYPE_STORM value
     * @property {number} WEATHER_TYPE_SNOW=4 WEATHER_TYPE_SNOW value
     */
    proto.WeatherType = (function() {
        const valuesById = {}, values = Object.create(valuesById);
        values[valuesById[0] = "WEATHER_TYPE_CLEAR"] = 0;
        values[valuesById[1] = "WEATHER_TYPE_RAIN"] = 1;
        values[valuesById[2] = "WEATHER_TYPE_FOG"] = 2;
        values[valuesById[3] = "WEATHER_TYPE_STORM"] = 3;
        values[valuesById[4] = "WEATHER_TYPE_SNOW"] = 4;
        return values;
    })();

    /**
     * ErrorCode enum.
     * @name proto.ErrorCode
     * @enum {number}
     * @property {number} ERROR_CODE_NONE=0 ERROR_CODE_NONE value
     * @property {number} ERROR_CODE_INVALID_REQUEST=1 ERROR_CODE_INVALID_REQUEST value
     * @property {number} ERROR_CODE_NOT_AUTHENTICATED=2 ERROR_CODE_NOT_AUTHENTICATED value
     * @property {number} ERROR_CODE_ENTITY_NOT_FOUND=3 ERROR_CODE_ENTITY_NOT_FOUND value
     * @property {number} ERROR_CODE_OUT_OF_RANGE=4 ERROR_CODE_OUT_OF_RANGE value
     * @property {number} ERROR_CODE_INSUFFICIENT_RESOURCES=5 ERROR_CODE_INSUFFICIENT_RESOURCES value
     * @property {number} ERROR_CODE_INVENTORY_FULL=6 ERROR_CODE_INVENTORY_FULL value
     * @property {number} ERROR_CODE_CANNOT_INTERACT=7 ERROR_CODE_CANNOT_INTERACT value
     * @property {number} ERROR_CODE_COOLDOWN_ACTIVE=8 ERROR_CODE_COOLDOWN_ACTIVE value
     * @property {number} ERROR_CODE_INSUFFICIENT_STAMINA=9 ERROR_CODE_INSUFFICIENT_STAMINA value
     * @property {number} ERROR_CODE_TARGET_INVALID=10 ERROR_CODE_TARGET_INVALID value
     * @property {number} ERROR_CODE_PATH_BLOCKED=11 ERROR_CODE_PATH_BLOCKED value
     * @property {number} ERROR_CODE_ALREADY_IN_PROGRESS=12 ERROR_CODE_ALREADY_IN_PROGRESS value
     * @property {number} ERROR_CODE_BUILDING_INCOMPLETE=13 ERROR_CODE_BUILDING_INCOMPLETE value
     * @property {number} ERROR_CODE_RECIPE_UNKNOWN=14 ERROR_CODE_RECIPE_UNKNOWN value
     */
    proto.ErrorCode = (function() {
        const valuesById = {}, values = Object.create(valuesById);
        values[valuesById[0] = "ERROR_CODE_NONE"] = 0;
        values[valuesById[1] = "ERROR_CODE_INVALID_REQUEST"] = 1;
        values[valuesById[2] = "ERROR_CODE_NOT_AUTHENTICATED"] = 2;
        values[valuesById[3] = "ERROR_CODE_ENTITY_NOT_FOUND"] = 3;
        values[valuesById[4] = "ERROR_CODE_OUT_OF_RANGE"] = 4;
        values[valuesById[5] = "ERROR_CODE_INSUFFICIENT_RESOURCES"] = 5;
        values[valuesById[6] = "ERROR_CODE_INVENTORY_FULL"] = 6;
        values[valuesById[7] = "ERROR_CODE_CANNOT_INTERACT"] = 7;
        values[valuesById[8] = "ERROR_CODE_COOLDOWN_ACTIVE"] = 8;
        values[valuesById[9] = "ERROR_CODE_INSUFFICIENT_STAMINA"] = 9;
        values[valuesById[10] = "ERROR_CODE_TARGET_INVALID"] = 10;
        values[valuesById[11] = "ERROR_CODE_PATH_BLOCKED"] = 11;
        values[valuesById[12] = "ERROR_CODE_ALREADY_IN_PROGRESS"] = 12;
        values[valuesById[13] = "ERROR_CODE_BUILDING_INCOMPLETE"] = 13;
        values[valuesById[14] = "ERROR_CODE_RECIPE_UNKNOWN"] = 14;
        return values;
    })();

    proto.Item = (function() {

        /**
         * Properties of an Item.
         * @memberof proto
         * @interface IItem
         * @property {number|null} [itemId] Item itemId
         * @property {string|null} [resource] Item resource
         * @property {number|null} [quality] Item quality
         * @property {number|null} [quantity] Item quantity
         * @property {number|null} [durability] Item durability
         */

        /**
         * Constructs a new Item.
         * @memberof proto
         * @classdesc Represents an Item.
         * @implements IItem
         * @constructor
         * @param {proto.IItem=} [properties] Properties to set
         */
        function Item(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Item itemId.
         * @member {number} itemId
         * @memberof proto.Item
         * @instance
         */
        Item.prototype.itemId = 0;

        /**
         * Item resource.
         * @member {string} resource
         * @memberof proto.Item
         * @instance
         */
        Item.prototype.resource = "";

        /**
         * Item quality.
         * @member {number} quality
         * @memberof proto.Item
         * @instance
         */
        Item.prototype.quality = 0;

        /**
         * Item quantity.
         * @member {number} quantity
         * @memberof proto.Item
         * @instance
         */
        Item.prototype.quantity = 0;

        /**
         * Item durability.
         * @member {number} durability
         * @memberof proto.Item
         * @instance
         */
        Item.prototype.durability = 0;

        /**
         * Creates a new Item instance using the specified properties.
         * @function create
         * @memberof proto.Item
         * @static
         * @param {proto.IItem=} [properties] Properties to set
         * @returns {proto.Item} Item instance
         */
        Item.create = function create(properties) {
            return new Item(properties);
        };

        /**
         * Encodes the specified Item message. Does not implicitly {@link proto.Item.verify|verify} messages.
         * @function encode
         * @memberof proto.Item
         * @static
         * @param {proto.IItem} message Item message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Item.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.itemId != null && Object.hasOwnProperty.call(message, "itemId"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.itemId);
            if (message.resource != null && Object.hasOwnProperty.call(message, "resource"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.resource);
            if (message.quality != null && Object.hasOwnProperty.call(message, "quality"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.quality);
            if (message.quantity != null && Object.hasOwnProperty.call(message, "quantity"))
                writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.quantity);
            if (message.durability != null && Object.hasOwnProperty.call(message, "durability"))
                writer.uint32(/* id 5, wireType 0 =*/40).uint32(message.durability);
            return writer;
        };

        /**
         * Encodes the specified Item message, length delimited. Does not implicitly {@link proto.Item.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.Item
         * @static
         * @param {proto.IItem} message Item message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Item.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an Item message from the specified reader or buffer.
         * @function decode
         * @memberof proto.Item
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.Item} Item
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Item.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.Item();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.itemId = reader.uint32();
                        break;
                    }
                case 2: {
                        message.resource = reader.string();
                        break;
                    }
                case 3: {
                        message.quality = reader.uint32();
                        break;
                    }
                case 4: {
                        message.quantity = reader.uint32();
                        break;
                    }
                case 5: {
                        message.durability = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an Item message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.Item
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.Item} Item
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Item.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an Item message.
         * @function verify
         * @memberof proto.Item
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Item.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.itemId != null && message.hasOwnProperty("itemId"))
                if (!$util.isInteger(message.itemId))
                    return "itemId: integer expected";
            if (message.resource != null && message.hasOwnProperty("resource"))
                if (!$util.isString(message.resource))
                    return "resource: string expected";
            if (message.quality != null && message.hasOwnProperty("quality"))
                if (!$util.isInteger(message.quality))
                    return "quality: integer expected";
            if (message.quantity != null && message.hasOwnProperty("quantity"))
                if (!$util.isInteger(message.quantity))
                    return "quantity: integer expected";
            if (message.durability != null && message.hasOwnProperty("durability"))
                if (!$util.isInteger(message.durability))
                    return "durability: integer expected";
            return null;
        };

        /**
         * Creates an Item message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.Item
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.Item} Item
         */
        Item.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.Item)
                return object;
            let message = new $root.proto.Item();
            if (object.itemId != null)
                message.itemId = object.itemId >>> 0;
            if (object.resource != null)
                message.resource = String(object.resource);
            if (object.quality != null)
                message.quality = object.quality >>> 0;
            if (object.quantity != null)
                message.quantity = object.quantity >>> 0;
            if (object.durability != null)
                message.durability = object.durability >>> 0;
            return message;
        };

        /**
         * Creates a plain object from an Item message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.Item
         * @static
         * @param {proto.Item} message Item
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Item.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.itemId = 0;
                object.resource = "";
                object.quality = 0;
                object.quantity = 0;
                object.durability = 0;
            }
            if (message.itemId != null && message.hasOwnProperty("itemId"))
                object.itemId = message.itemId;
            if (message.resource != null && message.hasOwnProperty("resource"))
                object.resource = message.resource;
            if (message.quality != null && message.hasOwnProperty("quality"))
                object.quality = message.quality;
            if (message.quantity != null && message.hasOwnProperty("quantity"))
                object.quantity = message.quantity;
            if (message.durability != null && message.hasOwnProperty("durability"))
                object.durability = message.durability;
            return object;
        };

        /**
         * Converts this Item to JSON.
         * @function toJSON
         * @memberof proto.Item
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Item.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Item
         * @function getTypeUrl
         * @memberof proto.Item
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Item.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.Item";
        };

        return Item;
    })();

    proto.InventorySlot = (function() {

        /**
         * Properties of an InventorySlot.
         * @memberof proto
         * @interface IInventorySlot
         * @property {number|null} [x] InventorySlot x
         * @property {number|null} [y] InventorySlot y
         * @property {proto.IItem|null} [item] InventorySlot item
         */

        /**
         * Constructs a new InventorySlot.
         * @memberof proto
         * @classdesc Represents an InventorySlot.
         * @implements IInventorySlot
         * @constructor
         * @param {proto.IInventorySlot=} [properties] Properties to set
         */
        function InventorySlot(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * InventorySlot x.
         * @member {number} x
         * @memberof proto.InventorySlot
         * @instance
         */
        InventorySlot.prototype.x = 0;

        /**
         * InventorySlot y.
         * @member {number} y
         * @memberof proto.InventorySlot
         * @instance
         */
        InventorySlot.prototype.y = 0;

        /**
         * InventorySlot item.
         * @member {proto.IItem|null|undefined} item
         * @memberof proto.InventorySlot
         * @instance
         */
        InventorySlot.prototype.item = null;

        /**
         * Creates a new InventorySlot instance using the specified properties.
         * @function create
         * @memberof proto.InventorySlot
         * @static
         * @param {proto.IInventorySlot=} [properties] Properties to set
         * @returns {proto.InventorySlot} InventorySlot instance
         */
        InventorySlot.create = function create(properties) {
            return new InventorySlot(properties);
        };

        /**
         * Encodes the specified InventorySlot message. Does not implicitly {@link proto.InventorySlot.verify|verify} messages.
         * @function encode
         * @memberof proto.InventorySlot
         * @static
         * @param {proto.IInventorySlot} message InventorySlot message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventorySlot.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.x != null && Object.hasOwnProperty.call(message, "x"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.x);
            if (message.y != null && Object.hasOwnProperty.call(message, "y"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.y);
            if (message.item != null && Object.hasOwnProperty.call(message, "item"))
                $root.proto.Item.encode(message.item, writer.uint32(/* id 3, wireType 2 =*/26).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified InventorySlot message, length delimited. Does not implicitly {@link proto.InventorySlot.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.InventorySlot
         * @static
         * @param {proto.IInventorySlot} message InventorySlot message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        InventorySlot.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an InventorySlot message from the specified reader or buffer.
         * @function decode
         * @memberof proto.InventorySlot
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.InventorySlot} InventorySlot
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventorySlot.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.InventorySlot();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.x = reader.uint32();
                        break;
                    }
                case 2: {
                        message.y = reader.uint32();
                        break;
                    }
                case 3: {
                        message.item = $root.proto.Item.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an InventorySlot message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.InventorySlot
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.InventorySlot} InventorySlot
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        InventorySlot.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an InventorySlot message.
         * @function verify
         * @memberof proto.InventorySlot
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        InventorySlot.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.x != null && message.hasOwnProperty("x"))
                if (!$util.isInteger(message.x))
                    return "x: integer expected";
            if (message.y != null && message.hasOwnProperty("y"))
                if (!$util.isInteger(message.y))
                    return "y: integer expected";
            if (message.item != null && message.hasOwnProperty("item")) {
                let error = $root.proto.Item.verify(message.item);
                if (error)
                    return "item." + error;
            }
            return null;
        };

        /**
         * Creates an InventorySlot message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.InventorySlot
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.InventorySlot} InventorySlot
         */
        InventorySlot.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.InventorySlot)
                return object;
            let message = new $root.proto.InventorySlot();
            if (object.x != null)
                message.x = object.x >>> 0;
            if (object.y != null)
                message.y = object.y >>> 0;
            if (object.item != null) {
                if (typeof object.item !== "object")
                    throw TypeError(".proto.InventorySlot.item: object expected");
                message.item = $root.proto.Item.fromObject(object.item);
            }
            return message;
        };

        /**
         * Creates a plain object from an InventorySlot message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.InventorySlot
         * @static
         * @param {proto.InventorySlot} message InventorySlot
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        InventorySlot.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.x = 0;
                object.y = 0;
                object.item = null;
            }
            if (message.x != null && message.hasOwnProperty("x"))
                object.x = message.x;
            if (message.y != null && message.hasOwnProperty("y"))
                object.y = message.y;
            if (message.item != null && message.hasOwnProperty("item"))
                object.item = $root.proto.Item.toObject(message.item, options);
            return object;
        };

        /**
         * Converts this InventorySlot to JSON.
         * @function toJSON
         * @memberof proto.InventorySlot
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        InventorySlot.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for InventorySlot
         * @function getTypeUrl
         * @memberof proto.InventorySlot
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        InventorySlot.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.InventorySlot";
        };

        return InventorySlot;
    })();

    proto.Inventory = (function() {

        /**
         * Properties of an Inventory.
         * @memberof proto
         * @interface IInventory
         * @property {number|null} [width] Inventory width
         * @property {number|null} [height] Inventory height
         * @property {Array.<proto.IInventorySlot>|null} [slots] Inventory slots
         */

        /**
         * Constructs a new Inventory.
         * @memberof proto
         * @classdesc Represents an Inventory.
         * @implements IInventory
         * @constructor
         * @param {proto.IInventory=} [properties] Properties to set
         */
        function Inventory(properties) {
            this.slots = [];
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Inventory width.
         * @member {number} width
         * @memberof proto.Inventory
         * @instance
         */
        Inventory.prototype.width = 0;

        /**
         * Inventory height.
         * @member {number} height
         * @memberof proto.Inventory
         * @instance
         */
        Inventory.prototype.height = 0;

        /**
         * Inventory slots.
         * @member {Array.<proto.IInventorySlot>} slots
         * @memberof proto.Inventory
         * @instance
         */
        Inventory.prototype.slots = $util.emptyArray;

        /**
         * Creates a new Inventory instance using the specified properties.
         * @function create
         * @memberof proto.Inventory
         * @static
         * @param {proto.IInventory=} [properties] Properties to set
         * @returns {proto.Inventory} Inventory instance
         */
        Inventory.create = function create(properties) {
            return new Inventory(properties);
        };

        /**
         * Encodes the specified Inventory message. Does not implicitly {@link proto.Inventory.verify|verify} messages.
         * @function encode
         * @memberof proto.Inventory
         * @static
         * @param {proto.IInventory} message Inventory message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Inventory.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.width != null && Object.hasOwnProperty.call(message, "width"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.width);
            if (message.height != null && Object.hasOwnProperty.call(message, "height"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.height);
            if (message.slots != null && message.slots.length)
                for (let i = 0; i < message.slots.length; ++i)
                    $root.proto.InventorySlot.encode(message.slots[i], writer.uint32(/* id 3, wireType 2 =*/26).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified Inventory message, length delimited. Does not implicitly {@link proto.Inventory.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.Inventory
         * @static
         * @param {proto.IInventory} message Inventory message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Inventory.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an Inventory message from the specified reader or buffer.
         * @function decode
         * @memberof proto.Inventory
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.Inventory} Inventory
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Inventory.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.Inventory();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.width = reader.uint32();
                        break;
                    }
                case 2: {
                        message.height = reader.uint32();
                        break;
                    }
                case 3: {
                        if (!(message.slots && message.slots.length))
                            message.slots = [];
                        message.slots.push($root.proto.InventorySlot.decode(reader, reader.uint32()));
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an Inventory message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.Inventory
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.Inventory} Inventory
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Inventory.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an Inventory message.
         * @function verify
         * @memberof proto.Inventory
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Inventory.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.width != null && message.hasOwnProperty("width"))
                if (!$util.isInteger(message.width))
                    return "width: integer expected";
            if (message.height != null && message.hasOwnProperty("height"))
                if (!$util.isInteger(message.height))
                    return "height: integer expected";
            if (message.slots != null && message.hasOwnProperty("slots")) {
                if (!Array.isArray(message.slots))
                    return "slots: array expected";
                for (let i = 0; i < message.slots.length; ++i) {
                    let error = $root.proto.InventorySlot.verify(message.slots[i]);
                    if (error)
                        return "slots." + error;
                }
            }
            return null;
        };

        /**
         * Creates an Inventory message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.Inventory
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.Inventory} Inventory
         */
        Inventory.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.Inventory)
                return object;
            let message = new $root.proto.Inventory();
            if (object.width != null)
                message.width = object.width >>> 0;
            if (object.height != null)
                message.height = object.height >>> 0;
            if (object.slots) {
                if (!Array.isArray(object.slots))
                    throw TypeError(".proto.Inventory.slots: array expected");
                message.slots = [];
                for (let i = 0; i < object.slots.length; ++i) {
                    if (typeof object.slots[i] !== "object")
                        throw TypeError(".proto.Inventory.slots: object expected");
                    message.slots[i] = $root.proto.InventorySlot.fromObject(object.slots[i]);
                }
            }
            return message;
        };

        /**
         * Creates a plain object from an Inventory message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.Inventory
         * @static
         * @param {proto.Inventory} message Inventory
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Inventory.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.arrays || options.defaults)
                object.slots = [];
            if (options.defaults) {
                object.width = 0;
                object.height = 0;
            }
            if (message.width != null && message.hasOwnProperty("width"))
                object.width = message.width;
            if (message.height != null && message.hasOwnProperty("height"))
                object.height = message.height;
            if (message.slots && message.slots.length) {
                object.slots = [];
                for (let j = 0; j < message.slots.length; ++j)
                    object.slots[j] = $root.proto.InventorySlot.toObject(message.slots[j], options);
            }
            return object;
        };

        /**
         * Converts this Inventory to JSON.
         * @function toJSON
         * @memberof proto.Inventory
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Inventory.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Inventory
         * @function getTypeUrl
         * @memberof proto.Inventory
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Inventory.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.Inventory";
        };

        return Inventory;
    })();

    proto.PaperdollSlot = (function() {

        /**
         * Properties of a PaperdollSlot.
         * @memberof proto
         * @interface IPaperdollSlot
         * @property {proto.EquipSlot|null} [slot] PaperdollSlot slot
         * @property {proto.IItem|null} [item] PaperdollSlot item
         */

        /**
         * Constructs a new PaperdollSlot.
         * @memberof proto
         * @classdesc Represents a PaperdollSlot.
         * @implements IPaperdollSlot
         * @constructor
         * @param {proto.IPaperdollSlot=} [properties] Properties to set
         */
        function PaperdollSlot(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * PaperdollSlot slot.
         * @member {proto.EquipSlot} slot
         * @memberof proto.PaperdollSlot
         * @instance
         */
        PaperdollSlot.prototype.slot = 0;

        /**
         * PaperdollSlot item.
         * @member {proto.IItem|null|undefined} item
         * @memberof proto.PaperdollSlot
         * @instance
         */
        PaperdollSlot.prototype.item = null;

        /**
         * Creates a new PaperdollSlot instance using the specified properties.
         * @function create
         * @memberof proto.PaperdollSlot
         * @static
         * @param {proto.IPaperdollSlot=} [properties] Properties to set
         * @returns {proto.PaperdollSlot} PaperdollSlot instance
         */
        PaperdollSlot.create = function create(properties) {
            return new PaperdollSlot(properties);
        };

        /**
         * Encodes the specified PaperdollSlot message. Does not implicitly {@link proto.PaperdollSlot.verify|verify} messages.
         * @function encode
         * @memberof proto.PaperdollSlot
         * @static
         * @param {proto.IPaperdollSlot} message PaperdollSlot message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        PaperdollSlot.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.slot != null && Object.hasOwnProperty.call(message, "slot"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.slot);
            if (message.item != null && Object.hasOwnProperty.call(message, "item"))
                $root.proto.Item.encode(message.item, writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified PaperdollSlot message, length delimited. Does not implicitly {@link proto.PaperdollSlot.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.PaperdollSlot
         * @static
         * @param {proto.IPaperdollSlot} message PaperdollSlot message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        PaperdollSlot.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a PaperdollSlot message from the specified reader or buffer.
         * @function decode
         * @memberof proto.PaperdollSlot
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.PaperdollSlot} PaperdollSlot
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        PaperdollSlot.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.PaperdollSlot();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.slot = reader.int32();
                        break;
                    }
                case 2: {
                        message.item = $root.proto.Item.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a PaperdollSlot message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.PaperdollSlot
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.PaperdollSlot} PaperdollSlot
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        PaperdollSlot.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a PaperdollSlot message.
         * @function verify
         * @memberof proto.PaperdollSlot
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        PaperdollSlot.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.slot != null && message.hasOwnProperty("slot"))
                switch (message.slot) {
                default:
                    return "slot: enum value expected";
                case 0:
                case 1:
                case 2:
                case 3:
                case 4:
                case 5:
                case 6:
                case 7:
                case 8:
                case 9:
                case 10:
                case 11:
                    break;
                }
            if (message.item != null && message.hasOwnProperty("item")) {
                let error = $root.proto.Item.verify(message.item);
                if (error)
                    return "item." + error;
            }
            return null;
        };

        /**
         * Creates a PaperdollSlot message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.PaperdollSlot
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.PaperdollSlot} PaperdollSlot
         */
        PaperdollSlot.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.PaperdollSlot)
                return object;
            let message = new $root.proto.PaperdollSlot();
            switch (object.slot) {
            default:
                if (typeof object.slot === "number") {
                    message.slot = object.slot;
                    break;
                }
                break;
            case "EQUIP_SLOT_NONE":
            case 0:
                message.slot = 0;
                break;
            case "EQUIP_SLOT_HEAD":
            case 1:
                message.slot = 1;
                break;
            case "EQUIP_SLOT_CHEST":
            case 2:
                message.slot = 2;
                break;
            case "EQUIP_SLOT_LEGS":
            case 3:
                message.slot = 3;
                break;
            case "EQUIP_SLOT_FEET":
            case 4:
                message.slot = 4;
                break;
            case "EQUIP_SLOT_HANDS":
            case 5:
                message.slot = 5;
                break;
            case "EQUIP_SLOT_LEFT_HAND":
            case 6:
                message.slot = 6;
                break;
            case "EQUIP_SLOT_RIGHT_HAND":
            case 7:
                message.slot = 7;
                break;
            case "EQUIP_SLOT_BACK":
            case 8:
                message.slot = 8;
                break;
            case "EQUIP_SLOT_NECK":
            case 9:
                message.slot = 9;
                break;
            case "EQUIP_SLOT_RING_1":
            case 10:
                message.slot = 10;
                break;
            case "EQUIP_SLOT_RING_2":
            case 11:
                message.slot = 11;
                break;
            }
            if (object.item != null) {
                if (typeof object.item !== "object")
                    throw TypeError(".proto.PaperdollSlot.item: object expected");
                message.item = $root.proto.Item.fromObject(object.item);
            }
            return message;
        };

        /**
         * Creates a plain object from a PaperdollSlot message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.PaperdollSlot
         * @static
         * @param {proto.PaperdollSlot} message PaperdollSlot
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        PaperdollSlot.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.slot = options.enums === String ? "EQUIP_SLOT_NONE" : 0;
                object.item = null;
            }
            if (message.slot != null && message.hasOwnProperty("slot"))
                object.slot = options.enums === String ? $root.proto.EquipSlot[message.slot] === undefined ? message.slot : $root.proto.EquipSlot[message.slot] : message.slot;
            if (message.item != null && message.hasOwnProperty("item"))
                object.item = $root.proto.Item.toObject(message.item, options);
            return object;
        };

        /**
         * Converts this PaperdollSlot to JSON.
         * @function toJSON
         * @memberof proto.PaperdollSlot
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        PaperdollSlot.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for PaperdollSlot
         * @function getTypeUrl
         * @memberof proto.PaperdollSlot
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        PaperdollSlot.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.PaperdollSlot";
        };

        return PaperdollSlot;
    })();

    proto.Paperdoll = (function() {

        /**
         * Properties of a Paperdoll.
         * @memberof proto
         * @interface IPaperdoll
         * @property {Array.<proto.IPaperdollSlot>|null} [slots] Paperdoll slots
         */

        /**
         * Constructs a new Paperdoll.
         * @memberof proto
         * @classdesc Represents a Paperdoll.
         * @implements IPaperdoll
         * @constructor
         * @param {proto.IPaperdoll=} [properties] Properties to set
         */
        function Paperdoll(properties) {
            this.slots = [];
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Paperdoll slots.
         * @member {Array.<proto.IPaperdollSlot>} slots
         * @memberof proto.Paperdoll
         * @instance
         */
        Paperdoll.prototype.slots = $util.emptyArray;

        /**
         * Creates a new Paperdoll instance using the specified properties.
         * @function create
         * @memberof proto.Paperdoll
         * @static
         * @param {proto.IPaperdoll=} [properties] Properties to set
         * @returns {proto.Paperdoll} Paperdoll instance
         */
        Paperdoll.create = function create(properties) {
            return new Paperdoll(properties);
        };

        /**
         * Encodes the specified Paperdoll message. Does not implicitly {@link proto.Paperdoll.verify|verify} messages.
         * @function encode
         * @memberof proto.Paperdoll
         * @static
         * @param {proto.IPaperdoll} message Paperdoll message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Paperdoll.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.slots != null && message.slots.length)
                for (let i = 0; i < message.slots.length; ++i)
                    $root.proto.PaperdollSlot.encode(message.slots[i], writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified Paperdoll message, length delimited. Does not implicitly {@link proto.Paperdoll.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.Paperdoll
         * @static
         * @param {proto.IPaperdoll} message Paperdoll message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Paperdoll.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Paperdoll message from the specified reader or buffer.
         * @function decode
         * @memberof proto.Paperdoll
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.Paperdoll} Paperdoll
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Paperdoll.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.Paperdoll();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        if (!(message.slots && message.slots.length))
                            message.slots = [];
                        message.slots.push($root.proto.PaperdollSlot.decode(reader, reader.uint32()));
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a Paperdoll message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.Paperdoll
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.Paperdoll} Paperdoll
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Paperdoll.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Paperdoll message.
         * @function verify
         * @memberof proto.Paperdoll
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Paperdoll.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.slots != null && message.hasOwnProperty("slots")) {
                if (!Array.isArray(message.slots))
                    return "slots: array expected";
                for (let i = 0; i < message.slots.length; ++i) {
                    let error = $root.proto.PaperdollSlot.verify(message.slots[i]);
                    if (error)
                        return "slots." + error;
                }
            }
            return null;
        };

        /**
         * Creates a Paperdoll message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.Paperdoll
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.Paperdoll} Paperdoll
         */
        Paperdoll.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.Paperdoll)
                return object;
            let message = new $root.proto.Paperdoll();
            if (object.slots) {
                if (!Array.isArray(object.slots))
                    throw TypeError(".proto.Paperdoll.slots: array expected");
                message.slots = [];
                for (let i = 0; i < object.slots.length; ++i) {
                    if (typeof object.slots[i] !== "object")
                        throw TypeError(".proto.Paperdoll.slots: object expected");
                    message.slots[i] = $root.proto.PaperdollSlot.fromObject(object.slots[i]);
                }
            }
            return message;
        };

        /**
         * Creates a plain object from a Paperdoll message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.Paperdoll
         * @static
         * @param {proto.Paperdoll} message Paperdoll
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Paperdoll.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.arrays || options.defaults)
                object.slots = [];
            if (message.slots && message.slots.length) {
                object.slots = [];
                for (let j = 0; j < message.slots.length; ++j)
                    object.slots[j] = $root.proto.PaperdollSlot.toObject(message.slots[j], options);
            }
            return object;
        };

        /**
         * Converts this Paperdoll to JSON.
         * @function toJSON
         * @memberof proto.Paperdoll
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Paperdoll.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Paperdoll
         * @function getTypeUrl
         * @memberof proto.Paperdoll
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Paperdoll.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.Paperdoll";
        };

        return Paperdoll;
    })();

    proto.EntityMovement = (function() {

        /**
         * Properties of an EntityMovement.
         * @memberof proto
         * @interface IEntityMovement
         * @property {proto.IPosition|null} [position] EntityMovement position
         * @property {proto.IVector2|null} [velocity] EntityMovement velocity
         * @property {proto.MoveMode|null} [moveMode] EntityMovement moveMode
         * @property {proto.IVector2|null} [targetPosition] EntityMovement targetPosition
         * @property {boolean|null} [isMoving] EntityMovement isMoving
         */

        /**
         * Constructs a new EntityMovement.
         * @memberof proto
         * @classdesc Represents an EntityMovement.
         * @implements IEntityMovement
         * @constructor
         * @param {proto.IEntityMovement=} [properties] Properties to set
         */
        function EntityMovement(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * EntityMovement position.
         * @member {proto.IPosition|null|undefined} position
         * @memberof proto.EntityMovement
         * @instance
         */
        EntityMovement.prototype.position = null;

        /**
         * EntityMovement velocity.
         * @member {proto.IVector2|null|undefined} velocity
         * @memberof proto.EntityMovement
         * @instance
         */
        EntityMovement.prototype.velocity = null;

        /**
         * EntityMovement moveMode.
         * @member {proto.MoveMode} moveMode
         * @memberof proto.EntityMovement
         * @instance
         */
        EntityMovement.prototype.moveMode = 0;

        /**
         * EntityMovement targetPosition.
         * @member {proto.IVector2|null|undefined} targetPosition
         * @memberof proto.EntityMovement
         * @instance
         */
        EntityMovement.prototype.targetPosition = null;

        /**
         * EntityMovement isMoving.
         * @member {boolean} isMoving
         * @memberof proto.EntityMovement
         * @instance
         */
        EntityMovement.prototype.isMoving = false;

        // OneOf field names bound to virtual getters and setters
        let $oneOfFields;

        // Virtual OneOf for proto3 optional field
        Object.defineProperty(EntityMovement.prototype, "_targetPosition", {
            get: $util.oneOfGetter($oneOfFields = ["targetPosition"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        /**
         * Creates a new EntityMovement instance using the specified properties.
         * @function create
         * @memberof proto.EntityMovement
         * @static
         * @param {proto.IEntityMovement=} [properties] Properties to set
         * @returns {proto.EntityMovement} EntityMovement instance
         */
        EntityMovement.create = function create(properties) {
            return new EntityMovement(properties);
        };

        /**
         * Encodes the specified EntityMovement message. Does not implicitly {@link proto.EntityMovement.verify|verify} messages.
         * @function encode
         * @memberof proto.EntityMovement
         * @static
         * @param {proto.IEntityMovement} message EntityMovement message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        EntityMovement.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.position != null && Object.hasOwnProperty.call(message, "position"))
                $root.proto.Position.encode(message.position, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            if (message.velocity != null && Object.hasOwnProperty.call(message, "velocity"))
                $root.proto.Vector2.encode(message.velocity, writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
            if (message.moveMode != null && Object.hasOwnProperty.call(message, "moveMode"))
                writer.uint32(/* id 3, wireType 0 =*/24).int32(message.moveMode);
            if (message.targetPosition != null && Object.hasOwnProperty.call(message, "targetPosition"))
                $root.proto.Vector2.encode(message.targetPosition, writer.uint32(/* id 4, wireType 2 =*/34).fork()).ldelim();
            if (message.isMoving != null && Object.hasOwnProperty.call(message, "isMoving"))
                writer.uint32(/* id 5, wireType 0 =*/40).bool(message.isMoving);
            return writer;
        };

        /**
         * Encodes the specified EntityMovement message, length delimited. Does not implicitly {@link proto.EntityMovement.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.EntityMovement
         * @static
         * @param {proto.IEntityMovement} message EntityMovement message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        EntityMovement.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an EntityMovement message from the specified reader or buffer.
         * @function decode
         * @memberof proto.EntityMovement
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.EntityMovement} EntityMovement
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        EntityMovement.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.EntityMovement();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.position = $root.proto.Position.decode(reader, reader.uint32());
                        break;
                    }
                case 2: {
                        message.velocity = $root.proto.Vector2.decode(reader, reader.uint32());
                        break;
                    }
                case 3: {
                        message.moveMode = reader.int32();
                        break;
                    }
                case 4: {
                        message.targetPosition = $root.proto.Vector2.decode(reader, reader.uint32());
                        break;
                    }
                case 5: {
                        message.isMoving = reader.bool();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an EntityMovement message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.EntityMovement
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.EntityMovement} EntityMovement
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        EntityMovement.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an EntityMovement message.
         * @function verify
         * @memberof proto.EntityMovement
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        EntityMovement.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            let properties = {};
            if (message.position != null && message.hasOwnProperty("position")) {
                let error = $root.proto.Position.verify(message.position);
                if (error)
                    return "position." + error;
            }
            if (message.velocity != null && message.hasOwnProperty("velocity")) {
                let error = $root.proto.Vector2.verify(message.velocity);
                if (error)
                    return "velocity." + error;
            }
            if (message.moveMode != null && message.hasOwnProperty("moveMode"))
                switch (message.moveMode) {
                default:
                    return "moveMode: enum value expected";
                case 0:
                case 1:
                case 2:
                case 3:
                    break;
                }
            if (message.targetPosition != null && message.hasOwnProperty("targetPosition")) {
                properties._targetPosition = 1;
                {
                    let error = $root.proto.Vector2.verify(message.targetPosition);
                    if (error)
                        return "targetPosition." + error;
                }
            }
            if (message.isMoving != null && message.hasOwnProperty("isMoving"))
                if (typeof message.isMoving !== "boolean")
                    return "isMoving: boolean expected";
            return null;
        };

        /**
         * Creates an EntityMovement message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.EntityMovement
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.EntityMovement} EntityMovement
         */
        EntityMovement.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.EntityMovement)
                return object;
            let message = new $root.proto.EntityMovement();
            if (object.position != null) {
                if (typeof object.position !== "object")
                    throw TypeError(".proto.EntityMovement.position: object expected");
                message.position = $root.proto.Position.fromObject(object.position);
            }
            if (object.velocity != null) {
                if (typeof object.velocity !== "object")
                    throw TypeError(".proto.EntityMovement.velocity: object expected");
                message.velocity = $root.proto.Vector2.fromObject(object.velocity);
            }
            switch (object.moveMode) {
            default:
                if (typeof object.moveMode === "number") {
                    message.moveMode = object.moveMode;
                    break;
                }
                break;
            case "MOVE_MODE_WALK":
            case 0:
                message.moveMode = 0;
                break;
            case "MOVE_MODE_RUN":
            case 1:
                message.moveMode = 1;
                break;
            case "MOVE_MODE_FAST_RUN":
            case 2:
                message.moveMode = 2;
                break;
            case "MOVE_MODE_SWIM":
            case 3:
                message.moveMode = 3;
                break;
            }
            if (object.targetPosition != null) {
                if (typeof object.targetPosition !== "object")
                    throw TypeError(".proto.EntityMovement.targetPosition: object expected");
                message.targetPosition = $root.proto.Vector2.fromObject(object.targetPosition);
            }
            if (object.isMoving != null)
                message.isMoving = Boolean(object.isMoving);
            return message;
        };

        /**
         * Creates a plain object from an EntityMovement message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.EntityMovement
         * @static
         * @param {proto.EntityMovement} message EntityMovement
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        EntityMovement.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.position = null;
                object.velocity = null;
                object.moveMode = options.enums === String ? "MOVE_MODE_WALK" : 0;
                object.isMoving = false;
            }
            if (message.position != null && message.hasOwnProperty("position"))
                object.position = $root.proto.Position.toObject(message.position, options);
            if (message.velocity != null && message.hasOwnProperty("velocity"))
                object.velocity = $root.proto.Vector2.toObject(message.velocity, options);
            if (message.moveMode != null && message.hasOwnProperty("moveMode"))
                object.moveMode = options.enums === String ? $root.proto.MoveMode[message.moveMode] === undefined ? message.moveMode : $root.proto.MoveMode[message.moveMode] : message.moveMode;
            if (message.targetPosition != null && message.hasOwnProperty("targetPosition")) {
                object.targetPosition = $root.proto.Vector2.toObject(message.targetPosition, options);
                if (options.oneofs)
                    object._targetPosition = "targetPosition";
            }
            if (message.isMoving != null && message.hasOwnProperty("isMoving"))
                object.isMoving = message.isMoving;
            return object;
        };

        /**
         * Converts this EntityMovement to JSON.
         * @function toJSON
         * @memberof proto.EntityMovement
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        EntityMovement.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for EntityMovement
         * @function getTypeUrl
         * @memberof proto.EntityMovement
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        EntityMovement.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.EntityMovement";
        };

        return EntityMovement;
    })();

    proto.PlayerFullState = (function() {

        /**
         * Properties of a PlayerFullState.
         * @memberof proto
         * @interface IPlayerFullState
         * @property {number|Long|null} [entityId] PlayerFullState entityId
         * @property {string|null} [name] PlayerFullState name
         * @property {proto.IEntityMovement|null} [movement] PlayerFullState movement
         * @property {proto.IInventory|null} [inventory] PlayerFullState inventory
         * @property {proto.IPaperdoll|null} [paperdoll] PlayerFullState paperdoll
         * @property {number|Long|null} [draggingEntityId] PlayerFullState draggingEntityId
         */

        /**
         * Constructs a new PlayerFullState.
         * @memberof proto
         * @classdesc Represents a PlayerFullState.
         * @implements IPlayerFullState
         * @constructor
         * @param {proto.IPlayerFullState=} [properties] Properties to set
         */
        function PlayerFullState(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * PlayerFullState entityId.
         * @member {number|Long} entityId
         * @memberof proto.PlayerFullState
         * @instance
         */
        PlayerFullState.prototype.entityId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * PlayerFullState name.
         * @member {string} name
         * @memberof proto.PlayerFullState
         * @instance
         */
        PlayerFullState.prototype.name = "";

        /**
         * PlayerFullState movement.
         * @member {proto.IEntityMovement|null|undefined} movement
         * @memberof proto.PlayerFullState
         * @instance
         */
        PlayerFullState.prototype.movement = null;

        /**
         * PlayerFullState inventory.
         * @member {proto.IInventory|null|undefined} inventory
         * @memberof proto.PlayerFullState
         * @instance
         */
        PlayerFullState.prototype.inventory = null;

        /**
         * PlayerFullState paperdoll.
         * @member {proto.IPaperdoll|null|undefined} paperdoll
         * @memberof proto.PlayerFullState
         * @instance
         */
        PlayerFullState.prototype.paperdoll = null;

        /**
         * PlayerFullState draggingEntityId.
         * @member {number|Long|null|undefined} draggingEntityId
         * @memberof proto.PlayerFullState
         * @instance
         */
        PlayerFullState.prototype.draggingEntityId = null;

        // OneOf field names bound to virtual getters and setters
        let $oneOfFields;

        // Virtual OneOf for proto3 optional field
        Object.defineProperty(PlayerFullState.prototype, "_draggingEntityId", {
            get: $util.oneOfGetter($oneOfFields = ["draggingEntityId"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        /**
         * Creates a new PlayerFullState instance using the specified properties.
         * @function create
         * @memberof proto.PlayerFullState
         * @static
         * @param {proto.IPlayerFullState=} [properties] Properties to set
         * @returns {proto.PlayerFullState} PlayerFullState instance
         */
        PlayerFullState.create = function create(properties) {
            return new PlayerFullState(properties);
        };

        /**
         * Encodes the specified PlayerFullState message. Does not implicitly {@link proto.PlayerFullState.verify|verify} messages.
         * @function encode
         * @memberof proto.PlayerFullState
         * @static
         * @param {proto.IPlayerFullState} message PlayerFullState message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        PlayerFullState.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.entityId != null && Object.hasOwnProperty.call(message, "entityId"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.entityId);
            if (message.name != null && Object.hasOwnProperty.call(message, "name"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.name);
            if (message.movement != null && Object.hasOwnProperty.call(message, "movement"))
                $root.proto.EntityMovement.encode(message.movement, writer.uint32(/* id 3, wireType 2 =*/26).fork()).ldelim();
            if (message.inventory != null && Object.hasOwnProperty.call(message, "inventory"))
                $root.proto.Inventory.encode(message.inventory, writer.uint32(/* id 6, wireType 2 =*/50).fork()).ldelim();
            if (message.paperdoll != null && Object.hasOwnProperty.call(message, "paperdoll"))
                $root.proto.Paperdoll.encode(message.paperdoll, writer.uint32(/* id 7, wireType 2 =*/58).fork()).ldelim();
            if (message.draggingEntityId != null && Object.hasOwnProperty.call(message, "draggingEntityId"))
                writer.uint32(/* id 8, wireType 0 =*/64).uint64(message.draggingEntityId);
            return writer;
        };

        /**
         * Encodes the specified PlayerFullState message, length delimited. Does not implicitly {@link proto.PlayerFullState.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.PlayerFullState
         * @static
         * @param {proto.IPlayerFullState} message PlayerFullState message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        PlayerFullState.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a PlayerFullState message from the specified reader or buffer.
         * @function decode
         * @memberof proto.PlayerFullState
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.PlayerFullState} PlayerFullState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        PlayerFullState.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.PlayerFullState();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.entityId = reader.uint64();
                        break;
                    }
                case 2: {
                        message.name = reader.string();
                        break;
                    }
                case 3: {
                        message.movement = $root.proto.EntityMovement.decode(reader, reader.uint32());
                        break;
                    }
                case 6: {
                        message.inventory = $root.proto.Inventory.decode(reader, reader.uint32());
                        break;
                    }
                case 7: {
                        message.paperdoll = $root.proto.Paperdoll.decode(reader, reader.uint32());
                        break;
                    }
                case 8: {
                        message.draggingEntityId = reader.uint64();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a PlayerFullState message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.PlayerFullState
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.PlayerFullState} PlayerFullState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        PlayerFullState.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a PlayerFullState message.
         * @function verify
         * @memberof proto.PlayerFullState
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        PlayerFullState.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            let properties = {};
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (!$util.isInteger(message.entityId) && !(message.entityId && $util.isInteger(message.entityId.low) && $util.isInteger(message.entityId.high)))
                    return "entityId: integer|Long expected";
            if (message.name != null && message.hasOwnProperty("name"))
                if (!$util.isString(message.name))
                    return "name: string expected";
            if (message.movement != null && message.hasOwnProperty("movement")) {
                let error = $root.proto.EntityMovement.verify(message.movement);
                if (error)
                    return "movement." + error;
            }
            if (message.inventory != null && message.hasOwnProperty("inventory")) {
                let error = $root.proto.Inventory.verify(message.inventory);
                if (error)
                    return "inventory." + error;
            }
            if (message.paperdoll != null && message.hasOwnProperty("paperdoll")) {
                let error = $root.proto.Paperdoll.verify(message.paperdoll);
                if (error)
                    return "paperdoll." + error;
            }
            if (message.draggingEntityId != null && message.hasOwnProperty("draggingEntityId")) {
                properties._draggingEntityId = 1;
                if (!$util.isInteger(message.draggingEntityId) && !(message.draggingEntityId && $util.isInteger(message.draggingEntityId.low) && $util.isInteger(message.draggingEntityId.high)))
                    return "draggingEntityId: integer|Long expected";
            }
            return null;
        };

        /**
         * Creates a PlayerFullState message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.PlayerFullState
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.PlayerFullState} PlayerFullState
         */
        PlayerFullState.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.PlayerFullState)
                return object;
            let message = new $root.proto.PlayerFullState();
            if (object.entityId != null)
                if ($util.Long)
                    (message.entityId = $util.Long.fromValue(object.entityId)).unsigned = true;
                else if (typeof object.entityId === "string")
                    message.entityId = parseInt(object.entityId, 10);
                else if (typeof object.entityId === "number")
                    message.entityId = object.entityId;
                else if (typeof object.entityId === "object")
                    message.entityId = new $util.LongBits(object.entityId.low >>> 0, object.entityId.high >>> 0).toNumber(true);
            if (object.name != null)
                message.name = String(object.name);
            if (object.movement != null) {
                if (typeof object.movement !== "object")
                    throw TypeError(".proto.PlayerFullState.movement: object expected");
                message.movement = $root.proto.EntityMovement.fromObject(object.movement);
            }
            if (object.inventory != null) {
                if (typeof object.inventory !== "object")
                    throw TypeError(".proto.PlayerFullState.inventory: object expected");
                message.inventory = $root.proto.Inventory.fromObject(object.inventory);
            }
            if (object.paperdoll != null) {
                if (typeof object.paperdoll !== "object")
                    throw TypeError(".proto.PlayerFullState.paperdoll: object expected");
                message.paperdoll = $root.proto.Paperdoll.fromObject(object.paperdoll);
            }
            if (object.draggingEntityId != null)
                if ($util.Long)
                    (message.draggingEntityId = $util.Long.fromValue(object.draggingEntityId)).unsigned = true;
                else if (typeof object.draggingEntityId === "string")
                    message.draggingEntityId = parseInt(object.draggingEntityId, 10);
                else if (typeof object.draggingEntityId === "number")
                    message.draggingEntityId = object.draggingEntityId;
                else if (typeof object.draggingEntityId === "object")
                    message.draggingEntityId = new $util.LongBits(object.draggingEntityId.low >>> 0, object.draggingEntityId.high >>> 0).toNumber(true);
            return message;
        };

        /**
         * Creates a plain object from a PlayerFullState message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.PlayerFullState
         * @static
         * @param {proto.PlayerFullState} message PlayerFullState
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        PlayerFullState.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.entityId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.entityId = options.longs === String ? "0" : 0;
                object.name = "";
                object.movement = null;
                object.inventory = null;
                object.paperdoll = null;
            }
            if (message.entityId != null && message.hasOwnProperty("entityId"))
                if (typeof message.entityId === "number")
                    object.entityId = options.longs === String ? String(message.entityId) : message.entityId;
                else
                    object.entityId = options.longs === String ? $util.Long.prototype.toString.call(message.entityId) : options.longs === Number ? new $util.LongBits(message.entityId.low >>> 0, message.entityId.high >>> 0).toNumber(true) : message.entityId;
            if (message.name != null && message.hasOwnProperty("name"))
                object.name = message.name;
            if (message.movement != null && message.hasOwnProperty("movement"))
                object.movement = $root.proto.EntityMovement.toObject(message.movement, options);
            if (message.inventory != null && message.hasOwnProperty("inventory"))
                object.inventory = $root.proto.Inventory.toObject(message.inventory, options);
            if (message.paperdoll != null && message.hasOwnProperty("paperdoll"))
                object.paperdoll = $root.proto.Paperdoll.toObject(message.paperdoll, options);
            if (message.draggingEntityId != null && message.hasOwnProperty("draggingEntityId")) {
                if (typeof message.draggingEntityId === "number")
                    object.draggingEntityId = options.longs === String ? String(message.draggingEntityId) : message.draggingEntityId;
                else
                    object.draggingEntityId = options.longs === String ? $util.Long.prototype.toString.call(message.draggingEntityId) : options.longs === Number ? new $util.LongBits(message.draggingEntityId.low >>> 0, message.draggingEntityId.high >>> 0).toNumber(true) : message.draggingEntityId;
                if (options.oneofs)
                    object._draggingEntityId = "draggingEntityId";
            }
            return object;
        };

        /**
         * Converts this PlayerFullState to JSON.
         * @function toJSON
         * @memberof proto.PlayerFullState
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        PlayerFullState.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for PlayerFullState
         * @function getTypeUrl
         * @memberof proto.PlayerFullState
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        PlayerFullState.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.PlayerFullState";
        };

        return PlayerFullState;
    })();

    proto.EntityAppearance = (function() {

        /**
         * Properties of an EntityAppearance.
         * @memberof proto
         * @interface IEntityAppearance
         * @property {string|null} [resource] EntityAppearance resource
         * @property {string|null} [name] EntityAppearance name
         */

        /**
         * Constructs a new EntityAppearance.
         * @memberof proto
         * @classdesc Represents an EntityAppearance.
         * @implements IEntityAppearance
         * @constructor
         * @param {proto.IEntityAppearance=} [properties] Properties to set
         */
        function EntityAppearance(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * EntityAppearance resource.
         * @member {string} resource
         * @memberof proto.EntityAppearance
         * @instance
         */
        EntityAppearance.prototype.resource = "";

        /**
         * EntityAppearance name.
         * @member {string} name
         * @memberof proto.EntityAppearance
         * @instance
         */
        EntityAppearance.prototype.name = "";

        /**
         * Creates a new EntityAppearance instance using the specified properties.
         * @function create
         * @memberof proto.EntityAppearance
         * @static
         * @param {proto.IEntityAppearance=} [properties] Properties to set
         * @returns {proto.EntityAppearance} EntityAppearance instance
         */
        EntityAppearance.create = function create(properties) {
            return new EntityAppearance(properties);
        };

        /**
         * Encodes the specified EntityAppearance message. Does not implicitly {@link proto.EntityAppearance.verify|verify} messages.
         * @function encode
         * @memberof proto.EntityAppearance
         * @static
         * @param {proto.IEntityAppearance} message EntityAppearance message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        EntityAppearance.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.resource != null && Object.hasOwnProperty.call(message, "resource"))
                writer.uint32(/* id 1, wireType 2 =*/10).string(message.resource);
            if (message.name != null && Object.hasOwnProperty.call(message, "name"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.name);
            return writer;
        };

        /**
         * Encodes the specified EntityAppearance message, length delimited. Does not implicitly {@link proto.EntityAppearance.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.EntityAppearance
         * @static
         * @param {proto.IEntityAppearance} message EntityAppearance message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        EntityAppearance.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an EntityAppearance message from the specified reader or buffer.
         * @function decode
         * @memberof proto.EntityAppearance
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.EntityAppearance} EntityAppearance
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        EntityAppearance.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.EntityAppearance();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.resource = reader.string();
                        break;
                    }
                case 2: {
                        message.name = reader.string();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an EntityAppearance message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.EntityAppearance
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.EntityAppearance} EntityAppearance
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        EntityAppearance.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an EntityAppearance message.
         * @function verify
         * @memberof proto.EntityAppearance
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        EntityAppearance.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.resource != null && message.hasOwnProperty("resource"))
                if (!$util.isString(message.resource))
                    return "resource: string expected";
            if (message.name != null && message.hasOwnProperty("name"))
                if (!$util.isString(message.name))
                    return "name: string expected";
            return null;
        };

        /**
         * Creates an EntityAppearance message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.EntityAppearance
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.EntityAppearance} EntityAppearance
         */
        EntityAppearance.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.EntityAppearance)
                return object;
            let message = new $root.proto.EntityAppearance();
            if (object.resource != null)
                message.resource = String(object.resource);
            if (object.name != null)
                message.name = String(object.name);
            return message;
        };

        /**
         * Creates a plain object from an EntityAppearance message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.EntityAppearance
         * @static
         * @param {proto.EntityAppearance} message EntityAppearance
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        EntityAppearance.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.resource = "";
                object.name = "";
            }
            if (message.resource != null && message.hasOwnProperty("resource"))
                object.resource = message.resource;
            if (message.name != null && message.hasOwnProperty("name"))
                object.name = message.name;
            return object;
        };

        /**
         * Converts this EntityAppearance to JSON.
         * @function toJSON
         * @memberof proto.EntityAppearance
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        EntityAppearance.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for EntityAppearance
         * @function getTypeUrl
         * @memberof proto.EntityAppearance
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        EntityAppearance.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.EntityAppearance";
        };

        return EntityAppearance;
    })();

    proto.ChunkCoord = (function() {

        /**
         * Properties of a ChunkCoord.
         * @memberof proto
         * @interface IChunkCoord
         * @property {number|null} [x] ChunkCoord x
         * @property {number|null} [y] ChunkCoord y
         */

        /**
         * Constructs a new ChunkCoord.
         * @memberof proto
         * @classdesc Represents a ChunkCoord.
         * @implements IChunkCoord
         * @constructor
         * @param {proto.IChunkCoord=} [properties] Properties to set
         */
        function ChunkCoord(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ChunkCoord x.
         * @member {number} x
         * @memberof proto.ChunkCoord
         * @instance
         */
        ChunkCoord.prototype.x = 0;

        /**
         * ChunkCoord y.
         * @member {number} y
         * @memberof proto.ChunkCoord
         * @instance
         */
        ChunkCoord.prototype.y = 0;

        /**
         * Creates a new ChunkCoord instance using the specified properties.
         * @function create
         * @memberof proto.ChunkCoord
         * @static
         * @param {proto.IChunkCoord=} [properties] Properties to set
         * @returns {proto.ChunkCoord} ChunkCoord instance
         */
        ChunkCoord.create = function create(properties) {
            return new ChunkCoord(properties);
        };

        /**
         * Encodes the specified ChunkCoord message. Does not implicitly {@link proto.ChunkCoord.verify|verify} messages.
         * @function encode
         * @memberof proto.ChunkCoord
         * @static
         * @param {proto.IChunkCoord} message ChunkCoord message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ChunkCoord.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.x != null && Object.hasOwnProperty.call(message, "x"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.x);
            if (message.y != null && Object.hasOwnProperty.call(message, "y"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.y);
            return writer;
        };

        /**
         * Encodes the specified ChunkCoord message, length delimited. Does not implicitly {@link proto.ChunkCoord.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.ChunkCoord
         * @static
         * @param {proto.IChunkCoord} message ChunkCoord message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ChunkCoord.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a ChunkCoord message from the specified reader or buffer.
         * @function decode
         * @memberof proto.ChunkCoord
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.ChunkCoord} ChunkCoord
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ChunkCoord.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.ChunkCoord();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.x = reader.int32();
                        break;
                    }
                case 2: {
                        message.y = reader.int32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a ChunkCoord message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.ChunkCoord
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.ChunkCoord} ChunkCoord
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ChunkCoord.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a ChunkCoord message.
         * @function verify
         * @memberof proto.ChunkCoord
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ChunkCoord.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.x != null && message.hasOwnProperty("x"))
                if (!$util.isInteger(message.x))
                    return "x: integer expected";
            if (message.y != null && message.hasOwnProperty("y"))
                if (!$util.isInteger(message.y))
                    return "y: integer expected";
            return null;
        };

        /**
         * Creates a ChunkCoord message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.ChunkCoord
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.ChunkCoord} ChunkCoord
         */
        ChunkCoord.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.ChunkCoord)
                return object;
            let message = new $root.proto.ChunkCoord();
            if (object.x != null)
                message.x = object.x | 0;
            if (object.y != null)
                message.y = object.y | 0;
            return message;
        };

        /**
         * Creates a plain object from a ChunkCoord message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.ChunkCoord
         * @static
         * @param {proto.ChunkCoord} message ChunkCoord
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ChunkCoord.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.x = 0;
                object.y = 0;
            }
            if (message.x != null && message.hasOwnProperty("x"))
                object.x = message.x;
            if (message.y != null && message.hasOwnProperty("y"))
                object.y = message.y;
            return object;
        };

        /**
         * Converts this ChunkCoord to JSON.
         * @function toJSON
         * @memberof proto.ChunkCoord
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ChunkCoord.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ChunkCoord
         * @function getTypeUrl
         * @memberof proto.ChunkCoord
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ChunkCoord.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.ChunkCoord";
        };

        return ChunkCoord;
    })();

    proto.ChunkData = (function() {

        /**
         * Properties of a ChunkData.
         * @memberof proto
         * @interface IChunkData
         * @property {proto.IChunkCoord|null} [coord] ChunkData coord
         * @property {Uint8Array|null} [tiles] ChunkData tiles
         * @property {number|null} [version] ChunkData version
         */

        /**
         * Constructs a new ChunkData.
         * @memberof proto
         * @classdesc Represents a ChunkData.
         * @implements IChunkData
         * @constructor
         * @param {proto.IChunkData=} [properties] Properties to set
         */
        function ChunkData(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ChunkData coord.
         * @member {proto.IChunkCoord|null|undefined} coord
         * @memberof proto.ChunkData
         * @instance
         */
        ChunkData.prototype.coord = null;

        /**
         * ChunkData tiles.
         * @member {Uint8Array} tiles
         * @memberof proto.ChunkData
         * @instance
         */
        ChunkData.prototype.tiles = $util.newBuffer([]);

        /**
         * ChunkData version.
         * @member {number} version
         * @memberof proto.ChunkData
         * @instance
         */
        ChunkData.prototype.version = 0;

        /**
         * Creates a new ChunkData instance using the specified properties.
         * @function create
         * @memberof proto.ChunkData
         * @static
         * @param {proto.IChunkData=} [properties] Properties to set
         * @returns {proto.ChunkData} ChunkData instance
         */
        ChunkData.create = function create(properties) {
            return new ChunkData(properties);
        };

        /**
         * Encodes the specified ChunkData message. Does not implicitly {@link proto.ChunkData.verify|verify} messages.
         * @function encode
         * @memberof proto.ChunkData
         * @static
         * @param {proto.IChunkData} message ChunkData message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ChunkData.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.coord != null && Object.hasOwnProperty.call(message, "coord"))
                $root.proto.ChunkCoord.encode(message.coord, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            if (message.tiles != null && Object.hasOwnProperty.call(message, "tiles"))
                writer.uint32(/* id 2, wireType 2 =*/18).bytes(message.tiles);
            if (message.version != null && Object.hasOwnProperty.call(message, "version"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.version);
            return writer;
        };

        /**
         * Encodes the specified ChunkData message, length delimited. Does not implicitly {@link proto.ChunkData.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.ChunkData
         * @static
         * @param {proto.IChunkData} message ChunkData message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ChunkData.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a ChunkData message from the specified reader or buffer.
         * @function decode
         * @memberof proto.ChunkData
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.ChunkData} ChunkData
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ChunkData.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.ChunkData();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.coord = $root.proto.ChunkCoord.decode(reader, reader.uint32());
                        break;
                    }
                case 2: {
                        message.tiles = reader.bytes();
                        break;
                    }
                case 3: {
                        message.version = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a ChunkData message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.ChunkData
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.ChunkData} ChunkData
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ChunkData.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a ChunkData message.
         * @function verify
         * @memberof proto.ChunkData
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ChunkData.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.coord != null && message.hasOwnProperty("coord")) {
                let error = $root.proto.ChunkCoord.verify(message.coord);
                if (error)
                    return "coord." + error;
            }
            if (message.tiles != null && message.hasOwnProperty("tiles"))
                if (!(message.tiles && typeof message.tiles.length === "number" || $util.isString(message.tiles)))
                    return "tiles: buffer expected";
            if (message.version != null && message.hasOwnProperty("version"))
                if (!$util.isInteger(message.version))
                    return "version: integer expected";
            return null;
        };

        /**
         * Creates a ChunkData message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.ChunkData
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.ChunkData} ChunkData
         */
        ChunkData.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.ChunkData)
                return object;
            let message = new $root.proto.ChunkData();
            if (object.coord != null) {
                if (typeof object.coord !== "object")
                    throw TypeError(".proto.ChunkData.coord: object expected");
                message.coord = $root.proto.ChunkCoord.fromObject(object.coord);
            }
            if (object.tiles != null)
                if (typeof object.tiles === "string")
                    $util.base64.decode(object.tiles, message.tiles = $util.newBuffer($util.base64.length(object.tiles)), 0);
                else if (object.tiles.length >= 0)
                    message.tiles = object.tiles;
            if (object.version != null)
                message.version = object.version >>> 0;
            return message;
        };

        /**
         * Creates a plain object from a ChunkData message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.ChunkData
         * @static
         * @param {proto.ChunkData} message ChunkData
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ChunkData.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.coord = null;
                if (options.bytes === String)
                    object.tiles = "";
                else {
                    object.tiles = [];
                    if (options.bytes !== Array)
                        object.tiles = $util.newBuffer(object.tiles);
                }
                object.version = 0;
            }
            if (message.coord != null && message.hasOwnProperty("coord"))
                object.coord = $root.proto.ChunkCoord.toObject(message.coord, options);
            if (message.tiles != null && message.hasOwnProperty("tiles"))
                object.tiles = options.bytes === String ? $util.base64.encode(message.tiles, 0, message.tiles.length) : options.bytes === Array ? Array.prototype.slice.call(message.tiles) : message.tiles;
            if (message.version != null && message.hasOwnProperty("version"))
                object.version = message.version;
            return object;
        };

        /**
         * Converts this ChunkData to JSON.
         * @function toJSON
         * @memberof proto.ChunkData
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ChunkData.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ChunkData
         * @function getTypeUrl
         * @memberof proto.ChunkData
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ChunkData.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.ChunkData";
        };

        return ChunkData;
    })();

    proto.C2S_Auth = (function() {

        /**
         * Properties of a C2S_Auth.
         * @memberof proto
         * @interface IC2S_Auth
         * @property {string|null} [token] C2S_Auth token
         * @property {string|null} [clientVersion] C2S_Auth clientVersion
         */

        /**
         * Constructs a new C2S_Auth.
         * @memberof proto
         * @classdesc Represents a C2S_Auth.
         * @implements IC2S_Auth
         * @constructor
         * @param {proto.IC2S_Auth=} [properties] Properties to set
         */
        function C2S_Auth(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * C2S_Auth token.
         * @member {string} token
         * @memberof proto.C2S_Auth
         * @instance
         */
        C2S_Auth.prototype.token = "";

        /**
         * C2S_Auth clientVersion.
         * @member {string} clientVersion
         * @memberof proto.C2S_Auth
         * @instance
         */
        C2S_Auth.prototype.clientVersion = "";

        /**
         * Creates a new C2S_Auth instance using the specified properties.
         * @function create
         * @memberof proto.C2S_Auth
         * @static
         * @param {proto.IC2S_Auth=} [properties] Properties to set
         * @returns {proto.C2S_Auth} C2S_Auth instance
         */
        C2S_Auth.create = function create(properties) {
            return new C2S_Auth(properties);
        };

        /**
         * Encodes the specified C2S_Auth message. Does not implicitly {@link proto.C2S_Auth.verify|verify} messages.
         * @function encode
         * @memberof proto.C2S_Auth
         * @static
         * @param {proto.IC2S_Auth} message C2S_Auth message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        C2S_Auth.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.token != null && Object.hasOwnProperty.call(message, "token"))
                writer.uint32(/* id 1, wireType 2 =*/10).string(message.token);
            if (message.clientVersion != null && Object.hasOwnProperty.call(message, "clientVersion"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.clientVersion);
            return writer;
        };

        /**
         * Encodes the specified C2S_Auth message, length delimited. Does not implicitly {@link proto.C2S_Auth.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.C2S_Auth
         * @static
         * @param {proto.IC2S_Auth} message C2S_Auth message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        C2S_Auth.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a C2S_Auth message from the specified reader or buffer.
         * @function decode
         * @memberof proto.C2S_Auth
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.C2S_Auth} C2S_Auth
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        C2S_Auth.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.C2S_Auth();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.token = reader.string();
                        break;
                    }
                case 2: {
                        message.clientVersion = reader.string();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a C2S_Auth message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.C2S_Auth
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.C2S_Auth} C2S_Auth
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        C2S_Auth.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a C2S_Auth message.
         * @function verify
         * @memberof proto.C2S_Auth
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        C2S_Auth.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.token != null && message.hasOwnProperty("token"))
                if (!$util.isString(message.token))
                    return "token: string expected";
            if (message.clientVersion != null && message.hasOwnProperty("clientVersion"))
                if (!$util.isString(message.clientVersion))
                    return "clientVersion: string expected";
            return null;
        };

        /**
         * Creates a C2S_Auth message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.C2S_Auth
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.C2S_Auth} C2S_Auth
         */
        C2S_Auth.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.C2S_Auth)
                return object;
            let message = new $root.proto.C2S_Auth();
            if (object.token != null)
                message.token = String(object.token);
            if (object.clientVersion != null)
                message.clientVersion = String(object.clientVersion);
            return message;
        };

        /**
         * Creates a plain object from a C2S_Auth message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.C2S_Auth
         * @static
         * @param {proto.C2S_Auth} message C2S_Auth
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        C2S_Auth.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.token = "";
                object.clientVersion = "";
            }
            if (message.token != null && message.hasOwnProperty("token"))
                object.token = message.token;
            if (message.clientVersion != null && message.hasOwnProperty("clientVersion"))
                object.clientVersion = message.clientVersion;
            return object;
        };

        /**
         * Converts this C2S_Auth to JSON.
         * @function toJSON
         * @memberof proto.C2S_Auth
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        C2S_Auth.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for C2S_Auth
         * @function getTypeUrl
         * @memberof proto.C2S_Auth
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        C2S_Auth.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.C2S_Auth";
        };

        return C2S_Auth;
    })();

    proto.C2S_Ping = (function() {

        /**
         * Properties of a C2S_Ping.
         * @memberof proto
         * @interface IC2S_Ping
         * @property {number|Long|null} [clientTimeMs] C2S_Ping clientTimeMs
         */

        /**
         * Constructs a new C2S_Ping.
         * @memberof proto
         * @classdesc Represents a C2S_Ping.
         * @implements IC2S_Ping
         * @constructor
         * @param {proto.IC2S_Ping=} [properties] Properties to set
         */
        function C2S_Ping(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * C2S_Ping clientTimeMs.
         * @member {number|Long} clientTimeMs
         * @memberof proto.C2S_Ping
         * @instance
         */
        C2S_Ping.prototype.clientTimeMs = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

        /**
         * Creates a new C2S_Ping instance using the specified properties.
         * @function create
         * @memberof proto.C2S_Ping
         * @static
         * @param {proto.IC2S_Ping=} [properties] Properties to set
         * @returns {proto.C2S_Ping} C2S_Ping instance
         */
        C2S_Ping.create = function create(properties) {
            return new C2S_Ping(properties);
        };

        /**
         * Encodes the specified C2S_Ping message. Does not implicitly {@link proto.C2S_Ping.verify|verify} messages.
         * @function encode
         * @memberof proto.C2S_Ping
         * @static
         * @param {proto.IC2S_Ping} message C2S_Ping message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        C2S_Ping.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.clientTimeMs != null && Object.hasOwnProperty.call(message, "clientTimeMs"))
                writer.uint32(/* id 1, wireType 0 =*/8).int64(message.clientTimeMs);
            return writer;
        };

        /**
         * Encodes the specified C2S_Ping message, length delimited. Does not implicitly {@link proto.C2S_Ping.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.C2S_Ping
         * @static
         * @param {proto.IC2S_Ping} message C2S_Ping message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        C2S_Ping.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a C2S_Ping message from the specified reader or buffer.
         * @function decode
         * @memberof proto.C2S_Ping
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.C2S_Ping} C2S_Ping
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        C2S_Ping.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.C2S_Ping();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.clientTimeMs = reader.int64();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a C2S_Ping message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.C2S_Ping
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.C2S_Ping} C2S_Ping
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        C2S_Ping.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a C2S_Ping message.
         * @function verify
         * @memberof proto.C2S_Ping
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        C2S_Ping.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.clientTimeMs != null && message.hasOwnProperty("clientTimeMs"))
                if (!$util.isInteger(message.clientTimeMs) && !(message.clientTimeMs && $util.isInteger(message.clientTimeMs.low) && $util.isInteger(message.clientTimeMs.high)))
                    return "clientTimeMs: integer|Long expected";
            return null;
        };

        /**
         * Creates a C2S_Ping message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.C2S_Ping
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.C2S_Ping} C2S_Ping
         */
        C2S_Ping.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.C2S_Ping)
                return object;
            let message = new $root.proto.C2S_Ping();
            if (object.clientTimeMs != null)
                if ($util.Long)
                    (message.clientTimeMs = $util.Long.fromValue(object.clientTimeMs)).unsigned = false;
                else if (typeof object.clientTimeMs === "string")
                    message.clientTimeMs = parseInt(object.clientTimeMs, 10);
                else if (typeof object.clientTimeMs === "number")
                    message.clientTimeMs = object.clientTimeMs;
                else if (typeof object.clientTimeMs === "object")
                    message.clientTimeMs = new $util.LongBits(object.clientTimeMs.low >>> 0, object.clientTimeMs.high >>> 0).toNumber();
            return message;
        };

        /**
         * Creates a plain object from a C2S_Ping message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.C2S_Ping
         * @static
         * @param {proto.C2S_Ping} message C2S_Ping
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        C2S_Ping.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                if ($util.Long) {
                    let long = new $util.Long(0, 0, false);
                    object.clientTimeMs = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.clientTimeMs = options.longs === String ? "0" : 0;
            if (message.clientTimeMs != null && message.hasOwnProperty("clientTimeMs"))
                if (typeof message.clientTimeMs === "number")
                    object.clientTimeMs = options.longs === String ? String(message.clientTimeMs) : message.clientTimeMs;
                else
                    object.clientTimeMs = options.longs === String ? $util.Long.prototype.toString.call(message.clientTimeMs) : options.longs === Number ? new $util.LongBits(message.clientTimeMs.low >>> 0, message.clientTimeMs.high >>> 0).toNumber() : message.clientTimeMs;
            return object;
        };

        /**
         * Converts this C2S_Ping to JSON.
         * @function toJSON
         * @memberof proto.C2S_Ping
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        C2S_Ping.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for C2S_Ping
         * @function getTypeUrl
         * @memberof proto.C2S_Ping
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        C2S_Ping.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.C2S_Ping";
        };

        return C2S_Ping;
    })();

    proto.ClientMessage = (function() {

        /**
         * Properties of a ClientMessage.
         * @memberof proto
         * @interface IClientMessage
         * @property {number|null} [sequence] ClientMessage sequence
         * @property {proto.IC2S_Auth|null} [auth] ClientMessage auth
         * @property {proto.IC2S_Ping|null} [ping] ClientMessage ping
         */

        /**
         * Constructs a new ClientMessage.
         * @memberof proto
         * @classdesc Represents a ClientMessage.
         * @implements IClientMessage
         * @constructor
         * @param {proto.IClientMessage=} [properties] Properties to set
         */
        function ClientMessage(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ClientMessage sequence.
         * @member {number} sequence
         * @memberof proto.ClientMessage
         * @instance
         */
        ClientMessage.prototype.sequence = 0;

        /**
         * ClientMessage auth.
         * @member {proto.IC2S_Auth|null|undefined} auth
         * @memberof proto.ClientMessage
         * @instance
         */
        ClientMessage.prototype.auth = null;

        /**
         * ClientMessage ping.
         * @member {proto.IC2S_Ping|null|undefined} ping
         * @memberof proto.ClientMessage
         * @instance
         */
        ClientMessage.prototype.ping = null;

        // OneOf field names bound to virtual getters and setters
        let $oneOfFields;

        /**
         * ClientMessage payload.
         * @member {"auth"|"ping"|undefined} payload
         * @memberof proto.ClientMessage
         * @instance
         */
        Object.defineProperty(ClientMessage.prototype, "payload", {
            get: $util.oneOfGetter($oneOfFields = ["auth", "ping"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        /**
         * Creates a new ClientMessage instance using the specified properties.
         * @function create
         * @memberof proto.ClientMessage
         * @static
         * @param {proto.IClientMessage=} [properties] Properties to set
         * @returns {proto.ClientMessage} ClientMessage instance
         */
        ClientMessage.create = function create(properties) {
            return new ClientMessage(properties);
        };

        /**
         * Encodes the specified ClientMessage message. Does not implicitly {@link proto.ClientMessage.verify|verify} messages.
         * @function encode
         * @memberof proto.ClientMessage
         * @static
         * @param {proto.IClientMessage} message ClientMessage message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ClientMessage.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.sequence != null && Object.hasOwnProperty.call(message, "sequence"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.sequence);
            if (message.auth != null && Object.hasOwnProperty.call(message, "auth"))
                $root.proto.C2S_Auth.encode(message.auth, writer.uint32(/* id 10, wireType 2 =*/82).fork()).ldelim();
            if (message.ping != null && Object.hasOwnProperty.call(message, "ping"))
                $root.proto.C2S_Ping.encode(message.ping, writer.uint32(/* id 11, wireType 2 =*/90).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified ClientMessage message, length delimited. Does not implicitly {@link proto.ClientMessage.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.ClientMessage
         * @static
         * @param {proto.IClientMessage} message ClientMessage message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ClientMessage.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a ClientMessage message from the specified reader or buffer.
         * @function decode
         * @memberof proto.ClientMessage
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.ClientMessage} ClientMessage
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ClientMessage.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.ClientMessage();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.sequence = reader.uint32();
                        break;
                    }
                case 10: {
                        message.auth = $root.proto.C2S_Auth.decode(reader, reader.uint32());
                        break;
                    }
                case 11: {
                        message.ping = $root.proto.C2S_Ping.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a ClientMessage message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.ClientMessage
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.ClientMessage} ClientMessage
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ClientMessage.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a ClientMessage message.
         * @function verify
         * @memberof proto.ClientMessage
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ClientMessage.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            let properties = {};
            if (message.sequence != null && message.hasOwnProperty("sequence"))
                if (!$util.isInteger(message.sequence))
                    return "sequence: integer expected";
            if (message.auth != null && message.hasOwnProperty("auth")) {
                properties.payload = 1;
                {
                    let error = $root.proto.C2S_Auth.verify(message.auth);
                    if (error)
                        return "auth." + error;
                }
            }
            if (message.ping != null && message.hasOwnProperty("ping")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.C2S_Ping.verify(message.ping);
                    if (error)
                        return "ping." + error;
                }
            }
            return null;
        };

        /**
         * Creates a ClientMessage message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.ClientMessage
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.ClientMessage} ClientMessage
         */
        ClientMessage.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.ClientMessage)
                return object;
            let message = new $root.proto.ClientMessage();
            if (object.sequence != null)
                message.sequence = object.sequence >>> 0;
            if (object.auth != null) {
                if (typeof object.auth !== "object")
                    throw TypeError(".proto.ClientMessage.auth: object expected");
                message.auth = $root.proto.C2S_Auth.fromObject(object.auth);
            }
            if (object.ping != null) {
                if (typeof object.ping !== "object")
                    throw TypeError(".proto.ClientMessage.ping: object expected");
                message.ping = $root.proto.C2S_Ping.fromObject(object.ping);
            }
            return message;
        };

        /**
         * Creates a plain object from a ClientMessage message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.ClientMessage
         * @static
         * @param {proto.ClientMessage} message ClientMessage
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ClientMessage.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.sequence = 0;
            if (message.sequence != null && message.hasOwnProperty("sequence"))
                object.sequence = message.sequence;
            if (message.auth != null && message.hasOwnProperty("auth")) {
                object.auth = $root.proto.C2S_Auth.toObject(message.auth, options);
                if (options.oneofs)
                    object.payload = "auth";
            }
            if (message.ping != null && message.hasOwnProperty("ping")) {
                object.ping = $root.proto.C2S_Ping.toObject(message.ping, options);
                if (options.oneofs)
                    object.payload = "ping";
            }
            return object;
        };

        /**
         * Converts this ClientMessage to JSON.
         * @function toJSON
         * @memberof proto.ClientMessage
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ClientMessage.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ClientMessage
         * @function getTypeUrl
         * @memberof proto.ClientMessage
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ClientMessage.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.ClientMessage";
        };

        return ClientMessage;
    })();

    proto.S2C_AuthResult = (function() {

        /**
         * Properties of a S2C_AuthResult.
         * @memberof proto
         * @interface IS2C_AuthResult
         * @property {boolean|null} [success] S2C_AuthResult success
         * @property {string|null} [errorMessage] S2C_AuthResult errorMessage
         * @property {number|Long|null} [playerEntityId] S2C_AuthResult playerEntityId
         * @property {proto.IPlayerFullState|null} [playerState] S2C_AuthResult playerState
         */

        /**
         * Constructs a new S2C_AuthResult.
         * @memberof proto
         * @classdesc Represents a S2C_AuthResult.
         * @implements IS2C_AuthResult
         * @constructor
         * @param {proto.IS2C_AuthResult=} [properties] Properties to set
         */
        function S2C_AuthResult(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_AuthResult success.
         * @member {boolean} success
         * @memberof proto.S2C_AuthResult
         * @instance
         */
        S2C_AuthResult.prototype.success = false;

        /**
         * S2C_AuthResult errorMessage.
         * @member {string} errorMessage
         * @memberof proto.S2C_AuthResult
         * @instance
         */
        S2C_AuthResult.prototype.errorMessage = "";

        /**
         * S2C_AuthResult playerEntityId.
         * @member {number|Long} playerEntityId
         * @memberof proto.S2C_AuthResult
         * @instance
         */
        S2C_AuthResult.prototype.playerEntityId = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * S2C_AuthResult playerState.
         * @member {proto.IPlayerFullState|null|undefined} playerState
         * @memberof proto.S2C_AuthResult
         * @instance
         */
        S2C_AuthResult.prototype.playerState = null;

        /**
         * Creates a new S2C_AuthResult instance using the specified properties.
         * @function create
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {proto.IS2C_AuthResult=} [properties] Properties to set
         * @returns {proto.S2C_AuthResult} S2C_AuthResult instance
         */
        S2C_AuthResult.create = function create(properties) {
            return new S2C_AuthResult(properties);
        };

        /**
         * Encodes the specified S2C_AuthResult message. Does not implicitly {@link proto.S2C_AuthResult.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {proto.IS2C_AuthResult} message S2C_AuthResult message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_AuthResult.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.success != null && Object.hasOwnProperty.call(message, "success"))
                writer.uint32(/* id 1, wireType 0 =*/8).bool(message.success);
            if (message.errorMessage != null && Object.hasOwnProperty.call(message, "errorMessage"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.errorMessage);
            if (message.playerEntityId != null && Object.hasOwnProperty.call(message, "playerEntityId"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.playerEntityId);
            if (message.playerState != null && Object.hasOwnProperty.call(message, "playerState"))
                $root.proto.PlayerFullState.encode(message.playerState, writer.uint32(/* id 4, wireType 2 =*/34).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified S2C_AuthResult message, length delimited. Does not implicitly {@link proto.S2C_AuthResult.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {proto.IS2C_AuthResult} message S2C_AuthResult message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_AuthResult.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_AuthResult message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_AuthResult} S2C_AuthResult
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_AuthResult.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_AuthResult();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.success = reader.bool();
                        break;
                    }
                case 2: {
                        message.errorMessage = reader.string();
                        break;
                    }
                case 3: {
                        message.playerEntityId = reader.uint64();
                        break;
                    }
                case 4: {
                        message.playerState = $root.proto.PlayerFullState.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_AuthResult message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_AuthResult} S2C_AuthResult
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_AuthResult.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_AuthResult message.
         * @function verify
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_AuthResult.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.success != null && message.hasOwnProperty("success"))
                if (typeof message.success !== "boolean")
                    return "success: boolean expected";
            if (message.errorMessage != null && message.hasOwnProperty("errorMessage"))
                if (!$util.isString(message.errorMessage))
                    return "errorMessage: string expected";
            if (message.playerEntityId != null && message.hasOwnProperty("playerEntityId"))
                if (!$util.isInteger(message.playerEntityId) && !(message.playerEntityId && $util.isInteger(message.playerEntityId.low) && $util.isInteger(message.playerEntityId.high)))
                    return "playerEntityId: integer|Long expected";
            if (message.playerState != null && message.hasOwnProperty("playerState")) {
                let error = $root.proto.PlayerFullState.verify(message.playerState);
                if (error)
                    return "playerState." + error;
            }
            return null;
        };

        /**
         * Creates a S2C_AuthResult message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_AuthResult} S2C_AuthResult
         */
        S2C_AuthResult.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_AuthResult)
                return object;
            let message = new $root.proto.S2C_AuthResult();
            if (object.success != null)
                message.success = Boolean(object.success);
            if (object.errorMessage != null)
                message.errorMessage = String(object.errorMessage);
            if (object.playerEntityId != null)
                if ($util.Long)
                    (message.playerEntityId = $util.Long.fromValue(object.playerEntityId)).unsigned = true;
                else if (typeof object.playerEntityId === "string")
                    message.playerEntityId = parseInt(object.playerEntityId, 10);
                else if (typeof object.playerEntityId === "number")
                    message.playerEntityId = object.playerEntityId;
                else if (typeof object.playerEntityId === "object")
                    message.playerEntityId = new $util.LongBits(object.playerEntityId.low >>> 0, object.playerEntityId.high >>> 0).toNumber(true);
            if (object.playerState != null) {
                if (typeof object.playerState !== "object")
                    throw TypeError(".proto.S2C_AuthResult.playerState: object expected");
                message.playerState = $root.proto.PlayerFullState.fromObject(object.playerState);
            }
            return message;
        };

        /**
         * Creates a plain object from a S2C_AuthResult message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {proto.S2C_AuthResult} message S2C_AuthResult
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_AuthResult.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.success = false;
                object.errorMessage = "";
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
                    object.playerEntityId = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.playerEntityId = options.longs === String ? "0" : 0;
                object.playerState = null;
            }
            if (message.success != null && message.hasOwnProperty("success"))
                object.success = message.success;
            if (message.errorMessage != null && message.hasOwnProperty("errorMessage"))
                object.errorMessage = message.errorMessage;
            if (message.playerEntityId != null && message.hasOwnProperty("playerEntityId"))
                if (typeof message.playerEntityId === "number")
                    object.playerEntityId = options.longs === String ? String(message.playerEntityId) : message.playerEntityId;
                else
                    object.playerEntityId = options.longs === String ? $util.Long.prototype.toString.call(message.playerEntityId) : options.longs === Number ? new $util.LongBits(message.playerEntityId.low >>> 0, message.playerEntityId.high >>> 0).toNumber(true) : message.playerEntityId;
            if (message.playerState != null && message.hasOwnProperty("playerState"))
                object.playerState = $root.proto.PlayerFullState.toObject(message.playerState, options);
            return object;
        };

        /**
         * Converts this S2C_AuthResult to JSON.
         * @function toJSON
         * @memberof proto.S2C_AuthResult
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_AuthResult.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_AuthResult
         * @function getTypeUrl
         * @memberof proto.S2C_AuthResult
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_AuthResult.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_AuthResult";
        };

        return S2C_AuthResult;
    })();

    proto.S2C_Pong = (function() {

        /**
         * Properties of a S2C_Pong.
         * @memberof proto
         * @interface IS2C_Pong
         * @property {number|Long|null} [clientTimeMs] S2C_Pong clientTimeMs
         * @property {number|Long|null} [serverTimeMs] S2C_Pong serverTimeMs
         */

        /**
         * Constructs a new S2C_Pong.
         * @memberof proto
         * @classdesc Represents a S2C_Pong.
         * @implements IS2C_Pong
         * @constructor
         * @param {proto.IS2C_Pong=} [properties] Properties to set
         */
        function S2C_Pong(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_Pong clientTimeMs.
         * @member {number|Long} clientTimeMs
         * @memberof proto.S2C_Pong
         * @instance
         */
        S2C_Pong.prototype.clientTimeMs = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

        /**
         * S2C_Pong serverTimeMs.
         * @member {number|Long} serverTimeMs
         * @memberof proto.S2C_Pong
         * @instance
         */
        S2C_Pong.prototype.serverTimeMs = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

        /**
         * Creates a new S2C_Pong instance using the specified properties.
         * @function create
         * @memberof proto.S2C_Pong
         * @static
         * @param {proto.IS2C_Pong=} [properties] Properties to set
         * @returns {proto.S2C_Pong} S2C_Pong instance
         */
        S2C_Pong.create = function create(properties) {
            return new S2C_Pong(properties);
        };

        /**
         * Encodes the specified S2C_Pong message. Does not implicitly {@link proto.S2C_Pong.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_Pong
         * @static
         * @param {proto.IS2C_Pong} message S2C_Pong message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_Pong.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.clientTimeMs != null && Object.hasOwnProperty.call(message, "clientTimeMs"))
                writer.uint32(/* id 1, wireType 0 =*/8).int64(message.clientTimeMs);
            if (message.serverTimeMs != null && Object.hasOwnProperty.call(message, "serverTimeMs"))
                writer.uint32(/* id 2, wireType 0 =*/16).int64(message.serverTimeMs);
            return writer;
        };

        /**
         * Encodes the specified S2C_Pong message, length delimited. Does not implicitly {@link proto.S2C_Pong.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_Pong
         * @static
         * @param {proto.IS2C_Pong} message S2C_Pong message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_Pong.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_Pong message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_Pong
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_Pong} S2C_Pong
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_Pong.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_Pong();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.clientTimeMs = reader.int64();
                        break;
                    }
                case 2: {
                        message.serverTimeMs = reader.int64();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_Pong message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_Pong
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_Pong} S2C_Pong
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_Pong.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_Pong message.
         * @function verify
         * @memberof proto.S2C_Pong
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_Pong.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.clientTimeMs != null && message.hasOwnProperty("clientTimeMs"))
                if (!$util.isInteger(message.clientTimeMs) && !(message.clientTimeMs && $util.isInteger(message.clientTimeMs.low) && $util.isInteger(message.clientTimeMs.high)))
                    return "clientTimeMs: integer|Long expected";
            if (message.serverTimeMs != null && message.hasOwnProperty("serverTimeMs"))
                if (!$util.isInteger(message.serverTimeMs) && !(message.serverTimeMs && $util.isInteger(message.serverTimeMs.low) && $util.isInteger(message.serverTimeMs.high)))
                    return "serverTimeMs: integer|Long expected";
            return null;
        };

        /**
         * Creates a S2C_Pong message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_Pong
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_Pong} S2C_Pong
         */
        S2C_Pong.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_Pong)
                return object;
            let message = new $root.proto.S2C_Pong();
            if (object.clientTimeMs != null)
                if ($util.Long)
                    (message.clientTimeMs = $util.Long.fromValue(object.clientTimeMs)).unsigned = false;
                else if (typeof object.clientTimeMs === "string")
                    message.clientTimeMs = parseInt(object.clientTimeMs, 10);
                else if (typeof object.clientTimeMs === "number")
                    message.clientTimeMs = object.clientTimeMs;
                else if (typeof object.clientTimeMs === "object")
                    message.clientTimeMs = new $util.LongBits(object.clientTimeMs.low >>> 0, object.clientTimeMs.high >>> 0).toNumber();
            if (object.serverTimeMs != null)
                if ($util.Long)
                    (message.serverTimeMs = $util.Long.fromValue(object.serverTimeMs)).unsigned = false;
                else if (typeof object.serverTimeMs === "string")
                    message.serverTimeMs = parseInt(object.serverTimeMs, 10);
                else if (typeof object.serverTimeMs === "number")
                    message.serverTimeMs = object.serverTimeMs;
                else if (typeof object.serverTimeMs === "object")
                    message.serverTimeMs = new $util.LongBits(object.serverTimeMs.low >>> 0, object.serverTimeMs.high >>> 0).toNumber();
            return message;
        };

        /**
         * Creates a plain object from a S2C_Pong message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_Pong
         * @static
         * @param {proto.S2C_Pong} message S2C_Pong
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_Pong.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                if ($util.Long) {
                    let long = new $util.Long(0, 0, false);
                    object.clientTimeMs = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.clientTimeMs = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    let long = new $util.Long(0, 0, false);
                    object.serverTimeMs = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.serverTimeMs = options.longs === String ? "0" : 0;
            }
            if (message.clientTimeMs != null && message.hasOwnProperty("clientTimeMs"))
                if (typeof message.clientTimeMs === "number")
                    object.clientTimeMs = options.longs === String ? String(message.clientTimeMs) : message.clientTimeMs;
                else
                    object.clientTimeMs = options.longs === String ? $util.Long.prototype.toString.call(message.clientTimeMs) : options.longs === Number ? new $util.LongBits(message.clientTimeMs.low >>> 0, message.clientTimeMs.high >>> 0).toNumber() : message.clientTimeMs;
            if (message.serverTimeMs != null && message.hasOwnProperty("serverTimeMs"))
                if (typeof message.serverTimeMs === "number")
                    object.serverTimeMs = options.longs === String ? String(message.serverTimeMs) : message.serverTimeMs;
                else
                    object.serverTimeMs = options.longs === String ? $util.Long.prototype.toString.call(message.serverTimeMs) : options.longs === Number ? new $util.LongBits(message.serverTimeMs.low >>> 0, message.serverTimeMs.high >>> 0).toNumber() : message.serverTimeMs;
            return object;
        };

        /**
         * Converts this S2C_Pong to JSON.
         * @function toJSON
         * @memberof proto.S2C_Pong
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_Pong.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_Pong
         * @function getTypeUrl
         * @memberof proto.S2C_Pong
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_Pong.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_Pong";
        };

        return S2C_Pong;
    })();

    proto.S2C_LoadChunk = (function() {

        /**
         * Properties of a S2C_LoadChunk.
         * @memberof proto
         * @interface IS2C_LoadChunk
         * @property {proto.IChunkData|null} [chunk] S2C_LoadChunk chunk
         */

        /**
         * Constructs a new S2C_LoadChunk.
         * @memberof proto
         * @classdesc Represents a S2C_LoadChunk.
         * @implements IS2C_LoadChunk
         * @constructor
         * @param {proto.IS2C_LoadChunk=} [properties] Properties to set
         */
        function S2C_LoadChunk(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_LoadChunk chunk.
         * @member {proto.IChunkData|null|undefined} chunk
         * @memberof proto.S2C_LoadChunk
         * @instance
         */
        S2C_LoadChunk.prototype.chunk = null;

        /**
         * Creates a new S2C_LoadChunk instance using the specified properties.
         * @function create
         * @memberof proto.S2C_LoadChunk
         * @static
         * @param {proto.IS2C_LoadChunk=} [properties] Properties to set
         * @returns {proto.S2C_LoadChunk} S2C_LoadChunk instance
         */
        S2C_LoadChunk.create = function create(properties) {
            return new S2C_LoadChunk(properties);
        };

        /**
         * Encodes the specified S2C_LoadChunk message. Does not implicitly {@link proto.S2C_LoadChunk.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_LoadChunk
         * @static
         * @param {proto.IS2C_LoadChunk} message S2C_LoadChunk message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_LoadChunk.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.chunk != null && Object.hasOwnProperty.call(message, "chunk"))
                $root.proto.ChunkData.encode(message.chunk, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified S2C_LoadChunk message, length delimited. Does not implicitly {@link proto.S2C_LoadChunk.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_LoadChunk
         * @static
         * @param {proto.IS2C_LoadChunk} message S2C_LoadChunk message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_LoadChunk.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_LoadChunk message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_LoadChunk
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_LoadChunk} S2C_LoadChunk
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_LoadChunk.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_LoadChunk();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.chunk = $root.proto.ChunkData.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_LoadChunk message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_LoadChunk
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_LoadChunk} S2C_LoadChunk
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_LoadChunk.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_LoadChunk message.
         * @function verify
         * @memberof proto.S2C_LoadChunk
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_LoadChunk.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.chunk != null && message.hasOwnProperty("chunk")) {
                let error = $root.proto.ChunkData.verify(message.chunk);
                if (error)
                    return "chunk." + error;
            }
            return null;
        };

        /**
         * Creates a S2C_LoadChunk message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_LoadChunk
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_LoadChunk} S2C_LoadChunk
         */
        S2C_LoadChunk.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_LoadChunk)
                return object;
            let message = new $root.proto.S2C_LoadChunk();
            if (object.chunk != null) {
                if (typeof object.chunk !== "object")
                    throw TypeError(".proto.S2C_LoadChunk.chunk: object expected");
                message.chunk = $root.proto.ChunkData.fromObject(object.chunk);
            }
            return message;
        };

        /**
         * Creates a plain object from a S2C_LoadChunk message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_LoadChunk
         * @static
         * @param {proto.S2C_LoadChunk} message S2C_LoadChunk
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_LoadChunk.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.chunk = null;
            if (message.chunk != null && message.hasOwnProperty("chunk"))
                object.chunk = $root.proto.ChunkData.toObject(message.chunk, options);
            return object;
        };

        /**
         * Converts this S2C_LoadChunk to JSON.
         * @function toJSON
         * @memberof proto.S2C_LoadChunk
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_LoadChunk.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_LoadChunk
         * @function getTypeUrl
         * @memberof proto.S2C_LoadChunk
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_LoadChunk.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_LoadChunk";
        };

        return S2C_LoadChunk;
    })();

    proto.S2C_UnloadChunk = (function() {

        /**
         * Properties of a S2C_UnloadChunk.
         * @memberof proto
         * @interface IS2C_UnloadChunk
         * @property {proto.IChunkCoord|null} [coord] S2C_UnloadChunk coord
         */

        /**
         * Constructs a new S2C_UnloadChunk.
         * @memberof proto
         * @classdesc Represents a S2C_UnloadChunk.
         * @implements IS2C_UnloadChunk
         * @constructor
         * @param {proto.IS2C_UnloadChunk=} [properties] Properties to set
         */
        function S2C_UnloadChunk(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * S2C_UnloadChunk coord.
         * @member {proto.IChunkCoord|null|undefined} coord
         * @memberof proto.S2C_UnloadChunk
         * @instance
         */
        S2C_UnloadChunk.prototype.coord = null;

        /**
         * Creates a new S2C_UnloadChunk instance using the specified properties.
         * @function create
         * @memberof proto.S2C_UnloadChunk
         * @static
         * @param {proto.IS2C_UnloadChunk=} [properties] Properties to set
         * @returns {proto.S2C_UnloadChunk} S2C_UnloadChunk instance
         */
        S2C_UnloadChunk.create = function create(properties) {
            return new S2C_UnloadChunk(properties);
        };

        /**
         * Encodes the specified S2C_UnloadChunk message. Does not implicitly {@link proto.S2C_UnloadChunk.verify|verify} messages.
         * @function encode
         * @memberof proto.S2C_UnloadChunk
         * @static
         * @param {proto.IS2C_UnloadChunk} message S2C_UnloadChunk message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_UnloadChunk.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.coord != null && Object.hasOwnProperty.call(message, "coord"))
                $root.proto.ChunkCoord.encode(message.coord, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified S2C_UnloadChunk message, length delimited. Does not implicitly {@link proto.S2C_UnloadChunk.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.S2C_UnloadChunk
         * @static
         * @param {proto.IS2C_UnloadChunk} message S2C_UnloadChunk message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        S2C_UnloadChunk.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a S2C_UnloadChunk message from the specified reader or buffer.
         * @function decode
         * @memberof proto.S2C_UnloadChunk
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.S2C_UnloadChunk} S2C_UnloadChunk
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_UnloadChunk.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.S2C_UnloadChunk();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.coord = $root.proto.ChunkCoord.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a S2C_UnloadChunk message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.S2C_UnloadChunk
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.S2C_UnloadChunk} S2C_UnloadChunk
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        S2C_UnloadChunk.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a S2C_UnloadChunk message.
         * @function verify
         * @memberof proto.S2C_UnloadChunk
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        S2C_UnloadChunk.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.coord != null && message.hasOwnProperty("coord")) {
                let error = $root.proto.ChunkCoord.verify(message.coord);
                if (error)
                    return "coord." + error;
            }
            return null;
        };

        /**
         * Creates a S2C_UnloadChunk message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.S2C_UnloadChunk
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.S2C_UnloadChunk} S2C_UnloadChunk
         */
        S2C_UnloadChunk.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.S2C_UnloadChunk)
                return object;
            let message = new $root.proto.S2C_UnloadChunk();
            if (object.coord != null) {
                if (typeof object.coord !== "object")
                    throw TypeError(".proto.S2C_UnloadChunk.coord: object expected");
                message.coord = $root.proto.ChunkCoord.fromObject(object.coord);
            }
            return message;
        };

        /**
         * Creates a plain object from a S2C_UnloadChunk message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.S2C_UnloadChunk
         * @static
         * @param {proto.S2C_UnloadChunk} message S2C_UnloadChunk
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        S2C_UnloadChunk.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.coord = null;
            if (message.coord != null && message.hasOwnProperty("coord"))
                object.coord = $root.proto.ChunkCoord.toObject(message.coord, options);
            return object;
        };

        /**
         * Converts this S2C_UnloadChunk to JSON.
         * @function toJSON
         * @memberof proto.S2C_UnloadChunk
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        S2C_UnloadChunk.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for S2C_UnloadChunk
         * @function getTypeUrl
         * @memberof proto.S2C_UnloadChunk
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        S2C_UnloadChunk.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.S2C_UnloadChunk";
        };

        return S2C_UnloadChunk;
    })();

    proto.ServerMessage = (function() {

        /**
         * Properties of a ServerMessage.
         * @memberof proto
         * @interface IServerMessage
         * @property {number|null} [sequence] ServerMessage sequence
         * @property {proto.IS2C_AuthResult|null} [authResult] ServerMessage authResult
         * @property {proto.IS2C_Pong|null} [pong] ServerMessage pong
         * @property {proto.IS2C_LoadChunk|null} [loadChunk] ServerMessage loadChunk
         * @property {proto.IS2C_UnloadChunk|null} [unloadChunk] ServerMessage unloadChunk
         */

        /**
         * Constructs a new ServerMessage.
         * @memberof proto
         * @classdesc Represents a ServerMessage.
         * @implements IServerMessage
         * @constructor
         * @param {proto.IServerMessage=} [properties] Properties to set
         */
        function ServerMessage(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ServerMessage sequence.
         * @member {number} sequence
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.sequence = 0;

        /**
         * ServerMessage authResult.
         * @member {proto.IS2C_AuthResult|null|undefined} authResult
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.authResult = null;

        /**
         * ServerMessage pong.
         * @member {proto.IS2C_Pong|null|undefined} pong
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.pong = null;

        /**
         * ServerMessage loadChunk.
         * @member {proto.IS2C_LoadChunk|null|undefined} loadChunk
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.loadChunk = null;

        /**
         * ServerMessage unloadChunk.
         * @member {proto.IS2C_UnloadChunk|null|undefined} unloadChunk
         * @memberof proto.ServerMessage
         * @instance
         */
        ServerMessage.prototype.unloadChunk = null;

        // OneOf field names bound to virtual getters and setters
        let $oneOfFields;

        /**
         * ServerMessage payload.
         * @member {"authResult"|"pong"|"loadChunk"|"unloadChunk"|undefined} payload
         * @memberof proto.ServerMessage
         * @instance
         */
        Object.defineProperty(ServerMessage.prototype, "payload", {
            get: $util.oneOfGetter($oneOfFields = ["authResult", "pong", "loadChunk", "unloadChunk"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        /**
         * Creates a new ServerMessage instance using the specified properties.
         * @function create
         * @memberof proto.ServerMessage
         * @static
         * @param {proto.IServerMessage=} [properties] Properties to set
         * @returns {proto.ServerMessage} ServerMessage instance
         */
        ServerMessage.create = function create(properties) {
            return new ServerMessage(properties);
        };

        /**
         * Encodes the specified ServerMessage message. Does not implicitly {@link proto.ServerMessage.verify|verify} messages.
         * @function encode
         * @memberof proto.ServerMessage
         * @static
         * @param {proto.IServerMessage} message ServerMessage message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ServerMessage.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.sequence != null && Object.hasOwnProperty.call(message, "sequence"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.sequence);
            if (message.authResult != null && Object.hasOwnProperty.call(message, "authResult"))
                $root.proto.S2C_AuthResult.encode(message.authResult, writer.uint32(/* id 10, wireType 2 =*/82).fork()).ldelim();
            if (message.pong != null && Object.hasOwnProperty.call(message, "pong"))
                $root.proto.S2C_Pong.encode(message.pong, writer.uint32(/* id 11, wireType 2 =*/90).fork()).ldelim();
            if (message.loadChunk != null && Object.hasOwnProperty.call(message, "loadChunk"))
                $root.proto.S2C_LoadChunk.encode(message.loadChunk, writer.uint32(/* id 12, wireType 2 =*/98).fork()).ldelim();
            if (message.unloadChunk != null && Object.hasOwnProperty.call(message, "unloadChunk"))
                $root.proto.S2C_UnloadChunk.encode(message.unloadChunk, writer.uint32(/* id 13, wireType 2 =*/106).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified ServerMessage message, length delimited. Does not implicitly {@link proto.ServerMessage.verify|verify} messages.
         * @function encodeDelimited
         * @memberof proto.ServerMessage
         * @static
         * @param {proto.IServerMessage} message ServerMessage message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ServerMessage.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a ServerMessage message from the specified reader or buffer.
         * @function decode
         * @memberof proto.ServerMessage
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {proto.ServerMessage} ServerMessage
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ServerMessage.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.proto.ServerMessage();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.sequence = reader.uint32();
                        break;
                    }
                case 10: {
                        message.authResult = $root.proto.S2C_AuthResult.decode(reader, reader.uint32());
                        break;
                    }
                case 11: {
                        message.pong = $root.proto.S2C_Pong.decode(reader, reader.uint32());
                        break;
                    }
                case 12: {
                        message.loadChunk = $root.proto.S2C_LoadChunk.decode(reader, reader.uint32());
                        break;
                    }
                case 13: {
                        message.unloadChunk = $root.proto.S2C_UnloadChunk.decode(reader, reader.uint32());
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a ServerMessage message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof proto.ServerMessage
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {proto.ServerMessage} ServerMessage
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ServerMessage.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a ServerMessage message.
         * @function verify
         * @memberof proto.ServerMessage
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ServerMessage.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            let properties = {};
            if (message.sequence != null && message.hasOwnProperty("sequence"))
                if (!$util.isInteger(message.sequence))
                    return "sequence: integer expected";
            if (message.authResult != null && message.hasOwnProperty("authResult")) {
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_AuthResult.verify(message.authResult);
                    if (error)
                        return "authResult." + error;
                }
            }
            if (message.pong != null && message.hasOwnProperty("pong")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_Pong.verify(message.pong);
                    if (error)
                        return "pong." + error;
                }
            }
            if (message.loadChunk != null && message.hasOwnProperty("loadChunk")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_LoadChunk.verify(message.loadChunk);
                    if (error)
                        return "loadChunk." + error;
                }
            }
            if (message.unloadChunk != null && message.hasOwnProperty("unloadChunk")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.proto.S2C_UnloadChunk.verify(message.unloadChunk);
                    if (error)
                        return "unloadChunk." + error;
                }
            }
            return null;
        };

        /**
         * Creates a ServerMessage message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof proto.ServerMessage
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {proto.ServerMessage} ServerMessage
         */
        ServerMessage.fromObject = function fromObject(object) {
            if (object instanceof $root.proto.ServerMessage)
                return object;
            let message = new $root.proto.ServerMessage();
            if (object.sequence != null)
                message.sequence = object.sequence >>> 0;
            if (object.authResult != null) {
                if (typeof object.authResult !== "object")
                    throw TypeError(".proto.ServerMessage.authResult: object expected");
                message.authResult = $root.proto.S2C_AuthResult.fromObject(object.authResult);
            }
            if (object.pong != null) {
                if (typeof object.pong !== "object")
                    throw TypeError(".proto.ServerMessage.pong: object expected");
                message.pong = $root.proto.S2C_Pong.fromObject(object.pong);
            }
            if (object.loadChunk != null) {
                if (typeof object.loadChunk !== "object")
                    throw TypeError(".proto.ServerMessage.loadChunk: object expected");
                message.loadChunk = $root.proto.S2C_LoadChunk.fromObject(object.loadChunk);
            }
            if (object.unloadChunk != null) {
                if (typeof object.unloadChunk !== "object")
                    throw TypeError(".proto.ServerMessage.unloadChunk: object expected");
                message.unloadChunk = $root.proto.S2C_UnloadChunk.fromObject(object.unloadChunk);
            }
            return message;
        };

        /**
         * Creates a plain object from a ServerMessage message. Also converts values to other types if specified.
         * @function toObject
         * @memberof proto.ServerMessage
         * @static
         * @param {proto.ServerMessage} message ServerMessage
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ServerMessage.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.sequence = 0;
            if (message.sequence != null && message.hasOwnProperty("sequence"))
                object.sequence = message.sequence;
            if (message.authResult != null && message.hasOwnProperty("authResult")) {
                object.authResult = $root.proto.S2C_AuthResult.toObject(message.authResult, options);
                if (options.oneofs)
                    object.payload = "authResult";
            }
            if (message.pong != null && message.hasOwnProperty("pong")) {
                object.pong = $root.proto.S2C_Pong.toObject(message.pong, options);
                if (options.oneofs)
                    object.payload = "pong";
            }
            if (message.loadChunk != null && message.hasOwnProperty("loadChunk")) {
                object.loadChunk = $root.proto.S2C_LoadChunk.toObject(message.loadChunk, options);
                if (options.oneofs)
                    object.payload = "loadChunk";
            }
            if (message.unloadChunk != null && message.hasOwnProperty("unloadChunk")) {
                object.unloadChunk = $root.proto.S2C_UnloadChunk.toObject(message.unloadChunk, options);
                if (options.oneofs)
                    object.payload = "unloadChunk";
            }
            return object;
        };

        /**
         * Converts this ServerMessage to JSON.
         * @function toJSON
         * @memberof proto.ServerMessage
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ServerMessage.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ServerMessage
         * @function getTypeUrl
         * @memberof proto.ServerMessage
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ServerMessage.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/proto.ServerMessage";
        };

        return ServerMessage;
    })();

    return proto;
})();

export { $root as default };
